package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bobg/errors"
	"github.com/bobg/folder/v3"
	"golang.org/x/time/rate"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func (maincmd) doAdd(ctx context.Context, fast bool, credsfile, tokenfile, user string, _ []string) error {
	con, err := newController(ctx, fast, rate.Inf, credsfile, tokenfile, user)
	if err != nil {
		return errors.Wrap(err, "creating controller")
	}

	msgID, err := con.handleMsgContent(ctx, os.Stdin)
	if err != nil {
		return errors.Wrap(err, "handling message content")
	}

	log.Printf("New message ID %s", msgID)

	return nil
}

func (maincmd) doAddClient(ctx context.Context, fast bool, privkeyFile, addURL, user string, _ []string) error {
	privkey, err := os.ReadFile(privkeyFile)
	if err != nil {
		return errors.Wrapf(err, "reading private key file %s", privkeyFile)
	}

	msg, err := io.ReadAll(os.Stdin)
	if err != nil {
		return errors.Wrap(err, "reading message from stdin")
	}

	sig := ed25519.Sign(privkey, msg)

	addReq := &addRequest{
		User:      user,
		Message:   msg,
		Signature: sig,
		Fast:      fast,
	}

	jreq, err := json.Marshal(addReq)
	if err != nil {
		return errors.Wrap(err, "marshalling request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addURL, bytes.NewReader(jreq))
	if err != nil {
		return errors.Wrap(err, "creating request")
	}
	req.Header.Set("Content-Type", "application/json")

	var cl http.Client
	resp, err := cl.Do(req)
	if err != nil {
		return errors.Wrap(err, "making request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, addURL)
	}

	var (
		addResp addResponse
		dec     = json.NewDecoder(resp.Body)
	)
	if err := dec.Decode(&addResp); err != nil {
		return errors.Wrap(err, "decoding response")
	}

	log.Printf("Added message with ID %s", addResp.MessageID)
	return nil
}

func (maincmd) doAddFrom(ctx context.Context, fast bool, rateDur time.Duration, credsfile, tokenfile, user string, args []string) error {
	if len(args) == 0 {
		return nil
	}

	con, err := newController(ctx, fast, rate.Every(rateDur), credsfile, tokenfile, user)
	if err != nil {
		return errors.Wrap(err, "creating controller")
	}

	for _, name := range args {
		if err := con.handleFolder(ctx, name); err != nil {
			return errors.Wrapf(err, "handling folder %s", name)
		}
	}

	return nil
}

func newController(ctx context.Context, fast bool, lim rate.Limit, credsfile, tokenfile, user string) (*controller, error) {
	if user == "" {
		return nil, fmt.Errorf("must supply a username with -user")
	}

	oauthClient, err := authenticate(ctx, credsfile, tokenfile)
	if err != nil {
		return nil, errors.Wrap(err, "authenticating")
	}

	svc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		return nil, errors.Wrap(err, "creating Gmail service")
	}

	return &controller{
		user:    user,
		msvc:    gmail.NewUsersMessagesService(svc),
		fast:    fast,
		limiter: rate.NewLimiter(lim, 1),
	}, nil
}

type controller struct {
	user    string
	msvc    *gmail.UsersMessagesService
	fast    bool
	limiter *rate.Limiter
}

func (c *controller) handleFolder(ctx context.Context, name string) error {
	log.Printf("Opening folder %s", name)

	f, err := folder.Open(name)
	if err != nil {
		return errors.Wrapf(err, "opening folder %s", name)
	}
	defer f.Close()

	for {
		msg, err := f.Message()
		if err != nil {
			return errors.Wrapf(err, "reading message from folder %s", name)
		}
		if msg == nil {
			return nil
		}
		if err := c.limiter.Wait(ctx); err != nil {
			return errors.Wrap(err, "waiting for rate limiter")
		}
		if err := c.handleMsg(ctx, msg); err != nil {
			log.Printf("Error handling message (will continue): %s", err)
		}
	}
}

func (c *controller) handleMsg(ctx context.Context, msg io.ReadCloser) error {
	defer msg.Close()

	msgID, err := c.handleMsgContent(ctx, msg)
	if err != nil {
		return errors.Wrap(err, "handling message content")
	}

	log.Printf("Added message with ID %s", msgID)

	return nil
}

func (c *controller) handleMsgContent(ctx context.Context, r io.Reader) (string, error) {
	inp, err := io.ReadAll(r)
	if err != nil {
		return "", errors.Wrap(err, "reading message")
	}
	inpMsg := &gmail.Message{
		Raw:      base64.URLEncoding.EncodeToString(inp),
		LabelIds: []string{"INBOX", "UNREAD"},
	}

	var doer doer
	if c.fast {
		call := c.msvc.Insert(c.user, inpMsg)
		call.InternalDateSource("dateHeader")
		call.Context(ctx)
		doer = call
	} else {
		call := c.msvc.Import(c.user, inpMsg)
		call.InternalDateSource("dateHeader")
		call.Context(ctx)
		doer = call
	}

	msg, err := doer.Do()
	if err != nil {
		return "", errors.Wrap(err, "adding message")
	}

	return msg.Id, nil
}

type doer interface {
	Do(opts ...googleapi.CallOption) (*gmail.Message, error)
}
