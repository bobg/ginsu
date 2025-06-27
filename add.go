package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
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

	return con.handleMsgContent(ctx, os.Stdin)
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

	return c.handleMsgContent(ctx, msg)
}

func (c *controller) handleMsgContent(ctx context.Context, r io.Reader) error {
	inp, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "reading message")
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
		return errors.Wrap(err, "adding message")
	}

	log.Printf("new message ID %s", msg.Id)
	return nil
}

type doer interface {
	Do(opts ...googleapi.CallOption) (*gmail.Message, error)
}
