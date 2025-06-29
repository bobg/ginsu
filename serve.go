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
		latest:    make(map[string]time.Time),
	}

	mux.Handle("POST /add", mid.JSON(s.addHandler))
	mux.Handle("GET /latest", mid.JSON(s.latestHandler))
	// mux.Handle("/auth", mid.Err(s.authHandler))
	mux.Handle("/healthz", mid.Err(s.healthzHandler))

	return s, nil
}

type server struct {
	*http.Server
	configdir string

	mu     sync.Mutex // protects the following
	latest map[string]time.Time
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

func (s *server) addHandler(ctx context.Context, addReq *addRequest) (*addResponse, error) {
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

	s.mu.Lock()
	s.latest[addReq.User] = time.Now()
	s.mu.Unlock()

	log.Printf("Added message with ID %s for user %s", msgID, addReq.User)

	resp := &addResponse{
		MessageID: msgID,
	}
	return resp, nil
}

func (s *server) latestHandler() (map[string]time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.latest, nil
}
