package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/bobg/basexx/v2"
	"github.com/bobg/errors"
	"github.com/bobg/mid"
	"golang.org/x/time/rate"
)

func (maincmd) doServe(ctx context.Context, addr, configdir string, _ []string) error {
	outerCtx := ctx

	s, err := newServer(ctx, addr, configdir)
	if err != nil {
		return errors.Wrap(err, "creating server")
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var listenAndServeErr error
	go func() {
		listenAndServeErr = s.ListenAndServe()
		if errors.Is(listenAndServeErr, http.ErrServerClosed) {
			log.Printf("Server closed")
			listenAndServeErr = nil
		}
	}()

	log.Printf("Listening on %s", addr)

	<-ctx.Done()
	err = s.Shutdown(outerCtx)

	return errors.Join(err, listenAndServeErr)
}

func newServer(ctx context.Context, addr, configdir string) (*server, error) {
	host := addr
	if strings.Contains(addr, ":") {
		var err error
		host, _, err = net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing address %s", addr)
		}
	}

	mux := http.NewServeMux()
	s := &server{
		Server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		configdir: configdir,
		info:      make(map[string]*userInfo),

		smtpHost: host+":smtp",
		smtpFrom: fmt.Sprintf("Ginsu <noreply@%s>", host),
		smtpAuth: nil,
	}

	mux.Handle("POST /add", mid.JSON(s.addHandler))
	mux.Handle("GET /info", mid.JSON(s.infoHandler))
	mux.Handle("/auth", http.HandlerFunc(authHandler))
	mux.Handle("/auth2", mid.Err(s.auth2Handler))
	mux.Handle("/auth3", mid.Err(s.auth3Handler))
	mux.Handle("/healthz", mid.Err(s.healthzHandler))

	return s, nil
}

type server struct {
	*http.Server
	configdir string
	smtpHost  string
	smtpFrom  string
	smtpAuth  smtp.Auth

	mu   sync.Mutex // protects the following
	info map[string]*userInfo
}

type userInfo struct {
	latestSuccess time.Time // last successful delivery
	latestFailure time.Time // last failed delivery
	numSuccesses  int
	numFailures   int
}

func (*server) healthzHandler(w http.ResponseWriter, _ *http.Request) error {
	_, err := fmt.Fprintf(w, "ok\n")
	return err
}

type addRequest struct {
	User      string `json:"user"`      // e.g. "ricky.ricardo@gmail.com"
	Message   []byte `json:"message"`   // the raw message content
	Signature []byte `json:"signature"` // the ed25519 signature of the message
	Fast      bool   `json:"fast"`
}

type addResponse struct {
	MessageID string `json:"message_id"`
}

func (s *server) addHandler(ctx context.Context, addReq *addRequest) (_ *addResponse, err error) {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		now := time.Now()

		inf, ok := s.info[addReq.User]
		if !ok {
			inf = &userInfo{}
			s.info[addReq.User] = inf
		}

		if err == nil {
			inf.latestSuccess = now
			inf.numSuccesses++
		} else {
			inf.latestFailure = now
			inf.numFailures++
			log.Printf("Error adding message for user %s: %s", addReq.User, err)
		}
	}()

	keyfile := filepath.Join(s.configdir, addReq.User, "pubkey")
	pubkey, err := os.ReadFile(keyfile)
	if err != nil {
		return nil, errors.Wrapf(err, "reading public key file %s", keyfile)
	}
	if !ed25519.Verify(ed25519.PublicKey(pubkey), addReq.Message, addReq.Signature) {
		return nil, mid.CodeErr{C: http.StatusForbidden}
	}

	var (
		credsfile = filepath.Join(s.configdir, addReq.User, "creds.json")
		tokenfile = filepath.Join(s.configdir, addReq.User, "token.json")
	)

	con, err := newController(ctx, addReq.Fast, rate.Inf, credsfile, tokenfile, addReq.User)
	if err != nil {
		return nil, errors.Wrap(err, "creating controller")
	}

	msgID, err := con.handleMsgContent(ctx, bytes.NewReader(addReq.Message))
	if err != nil {
		return nil, errors.Wrap(err, "handling message content")
	}

	log.Printf("Added message with ID %s for user %s", msgID, addReq.User)

	resp := &addResponse{
		MessageID: msgID,
	}
	return resp, nil
}

type jinfo struct {
	LatestSuccess string `json:"latest_success,omitempty"`
	LatestFailure string `json:"latest_failure,omitempty"`
	NumSuccesses  int    `json:"num_successes"`
	NumFailures   int    `json:"num_failures"`
}

func (s *server) infoHandler() map[string]jinfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		now    = time.Now()
		jinfos = make(map[string]jinfo, len(s.info))
	)
	for user, inf := range s.info {
		j := jinfo{
			NumSuccesses: inf.numSuccesses,
			NumFailures:  inf.numFailures,
		}
		if !inf.latestSuccess.IsZero() {
			j.LatestSuccess = now.Sub(inf.latestSuccess).Round(time.Second).String() + " ago"
		}
		if !inf.latestFailure.IsZero() {
			j.LatestFailure = now.Sub(inf.latestFailure).Round(time.Second).String() + " ago"
		}
		jinfos[user] = j
	}
	return jinfos
}

func authHandler(w http.ResponseWriter, req *http.Request) {
	http.ServeFileFS(w, req, templates, "templates/auth.html")
}

func (s *server) auth2Handler(w http.ResponseWriter, req *http.Request) error {
	addr := req.FormValue("addr")
	addr = strings.TrimSpace(addr)
	addr = strings.ToLower(addr)
	if addr == "" {
		return mid.CodeErr{C: http.StatusBadRequest}
	}

	http.ServeFileFS(w, req, templates, "templates/auth2.html")

	userDir := filepath.Join(s.configdir, addr)
	_, err := os.Stat(userDir)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf("Config directory for user %s does not exist", addr)
		return nil
	}

	var (
		now        = time.Now()
		vtokenFile = filepath.Join(userDir, "vtoken.json")
	)

	info, err := os.Stat(vtokenFile)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// ok
	case err != nil:
		log.Printf("Error statting vtoken file %s: %s", vtokenFile, err)
		return nil
	case time.Since(info.ModTime()) < authHysteresis:
		log.Printf("File %s exists and is recent, not re-issuing", vtokenFile)
		return nil
	}

	var (
		exp     = now.Add(expDur)
		expSecs = exp.Unix()
		nonce   [nonceBytes]byte
		secret  [secretBytes]byte
	)

	if _, err := rand.Read(secret[:]); err != nil {
		return errors.Wrap(err, "generating secret")
	}
	if _, err := rand.Read(nonce[:]); err != nil {
		return errors.Wrap(err, "generating nonce")
	}
	vt := vtoken{
		ExpSecs: expSecs,
		Nonce:   nonce[:],
		Secret:  secret[:],
	}
	j, err := json.Marshal(vt)
	if err != nil {
		log.Printf("Error marshaling vtoken: %s", err)
		return nil
	}
	if err := os.WriteFile(vtokenFile, j, 0644); err != nil {
		log.Printf("Error writing vtoken file %s: %s", vtokenFile, err)
		return nil
	}

	h := hmac.New(sha256.New, secret[:])
	if err := binary.Write(h, binary.LittleEndian, expSecs); err != nil {
		log.Printf("Error hashing expiration time: %s", err)
		return nil
	}
	if _, err := h.Write(nonce[:]); err != nil {
		log.Printf("Error hashing nonce: %s", err)
		return nil
	}
	token := h.Sum(nil)

	tokenStr, err := basexx.Convert(string(token[:]), basexx.Binary, basexx.Base50)
	if err != nil {
		log.Printf("Error converting token to Base50: %s", err)
		return nil
	}

	nonceStr, err := basexx.Convert(string(nonce[:]), basexx.Binary, basexx.Base50)
	if err != nil {
		log.Printf("Error converting nonce to Base50: %s", err)
		return nil
	}

	v := url.Values{
		"addr":  []string{addr},
		"exp":   []string{fmt.Sprintf("%d", expSecs)},
		"nonce": []string{nonceStr},
		"token": []string{tokenStr},
	}

	type tmplData struct {
		Addr, Link string
	}
	data := tmplData{
		Addr: addr,
		Link: fmt.Sprintf("https://%s/auth3?%s", s.Addr, v.Encode()),
	}

	log.Printf("Sending link message with addr %s, link %s", data.Addr, data.Link)

	var buf bytes.Buffer
	if err := authMailTmpl.Execute(&buf, data); err != nil {
		log.Printf("Error executing auth mail template: %s", err)
		return nil
	}
	authMail := buf.Bytes()

	if err := smtp.SendMail(s.smtpHost, s.smtpAuth, s.smtpFrom, []string{addr}, authMail); err != nil {
		log.Printf("Error sending auth mail to %s: %s", addr, err)
		return nil
	}

	return nil
}

var authMailTmpl = template.Must(template.ParseFS(templates, "templates/authmail.tmpl"))

const (
	secretBytes    = 32
	nonceBytes     = 15 // to make base64 encoding come out nicely
	expDur         = time.Hour
	authHysteresis = 5 * time.Minute // how long to wait before re-issuing a vtoken
)

// TODO: we only need to store the secret
type vtoken struct {
	ExpSecs int64  `json:"exp_secs"` // expiration time in Unix seconds
	Nonce   []byte `json:"nonce"`    // random nonce
	Secret  []byte `json:"secret"`   // random secret
}

func (s *server) auth3Handler(w http.ResponseWriter, req *http.Request) error {
	var (
		addr       = req.FormValue("addr")
		expSecsStr = req.FormValue("exp")
		nonceStr   = req.FormValue("nonce")
		tokenStr   = req.FormValue("token")
	)

	expSecs, err := strconv.ParseInt(expSecsStr, 10, 64)
	if err != nil {
		return mid.CodeErr{C: http.StatusBadRequest, Err: errors.Wrap(err, "parsing expiration time")}
	}
	exp := time.Unix(expSecs, 0)
	if time.Now().After(exp) {
		return mid.CodeErr{C: http.StatusBadRequest, Err: errors.New("token has expired")}
	}

	nonce, err := basexx.Convert(nonceStr, basexx.Base50, basexx.Binary)
	if err != nil {
		err = errors.Wrap(err, "converting nonce from Base50")
		return mid.CodeErr{C: http.StatusBadRequest, Err: err}
	}

	token, err := basexx.Convert(tokenStr, basexx.Base50, basexx.Binary)
	if err != nil {
		err = errors.Wrap(err, "converting token from Base50")
		return mid.CodeErr{C: http.StatusBadRequest, Err: err}
	}

	var (
		userDir    = filepath.Join(s.configdir, addr)
		vtokenFile = filepath.Join(userDir, "vtoken.json")
	)
	vtokenBytes, err := os.ReadFile(vtokenFile)
	if errors.Is(err, os.ErrNotExist) {
		return mid.CodeErr{C: http.StatusForbidden}
	}
	if err != nil {
		return errors.Wrapf(err, "reading vtoken file %s", vtokenFile)
	}
	var vt vtoken
	if err := json.Unmarshal(vtokenBytes, &vt); err != nil {
		return errors.Wrapf(err, "unmarshalling vtoken from %s", vtokenFile)
	}

	h := hmac.New(sha256.New, vt.Secret)
	if err := binary.Write(h, binary.LittleEndian, expSecs); err != nil {
		return errors.Wrap(err, "hashing expiration time")
	}

	if _, err := h.Write([]byte(nonce)); err != nil {
		return errors.Wrap(err, "hashing nonce")
	}

	var (
		got  = h.Sum(nil)
		want = []byte(token)
	)

	if !hmac.Equal(got, want) {
		return mid.CodeErr{C: http.StatusForbidden, Err: errors.New("invalid token")}
	}

	var (
		credsfile = filepath.Join(s.configdir, addr, "creds.json")
		tokenfile = filepath.Join(s.configdir, addr, "token.json")
		ctx       = req.Context()
	)

	if _, err := authenticate(ctx, credsfile, tokenfile); err != nil {
		return errors.Wrap(err, "authenticating")
	}

	http.ServeFileFS(w, req, templates, "templates/auth3.html")

	return nil
}

//go:embed templates
var templates embed.FS
