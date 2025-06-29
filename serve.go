package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

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
			listenAndServeErr = nil
		}
	}()

	log.Printf("Listening on %s", addr)

	<-ctx.Done()
	err = s.Shutdown(outerCtx)

	return errors.Join(err, listenAndServeErr)
}

func newServer(ctx context.Context, addr, configdir string) (*server, error) {
	mux := http.NewServeMux()
	s := &server{
		Server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		configdir: configdir,
		info:      make(map[string]*userInfo),
	}

	mux.Handle("POST /add", mid.JSON(s.addHandler))
	mux.Handle("GET /info", mid.JSON(s.infoHandler))
	// mux.Handle("/auth", mid.Err(s.authHandler))
	mux.Handle("/healthz", mid.Err(s.healthzHandler))

	return s, nil
}

type server struct {
	*http.Server
	configdir string

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
