// Command ginsu is a Gmail INSerter for U.
// It accepts an e-mail message on standard input
// and uses the Gmail API to "insert" or "import" it.
// Alternatively, it reads the names of mail folders from the command line,
// parses messages out of those,
// and inserts or imports them.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/bobg/folder/v3"
	"github.com/bobg/oauther/v5"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type doer interface {
	Do(opts ...googleapi.CallOption) (*gmail.Message, error)
}

func main() {
	var (
		credsfile, tokenfile string
		user                 string
		mode                 string // "import", "insert", "auth"
		rate                 time.Duration
	)
	flag.StringVar(&credsfile, "creds", "creds.json", "path to credentials file")
	flag.StringVar(&mode, "mode", "", "mode (import, insert, or auth)")
	flag.DurationVar(&rate, "rate", 100*time.Millisecond, "rate limit in folder-parsing mode")
	flag.StringVar(&tokenfile, "token", "token.json", "token cache file")
	flag.StringVar(&user, "user", "", "Gmail user ID")

	flag.Parse()

	creds, err := os.ReadFile(credsfile)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	switch mode {
	case "import", "insert", "auth":
		// do nothing
	default:
		log.Fatal("Must specify -mode, one of import, insert, or auth")
	}

	if user == "" && mode != "auth" {
		log.Fatal("Must supply a username with -user")
	}

	oauthClient, err := oauther.Client(ctx, tokenfile, creds, gmail.GmailInsertScope)
	if err != nil {
		log.Fatal(err)
	}

	if mode == "auth" {
		fmt.Println("Auth done")
		return
	}

	svc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		log.Fatal(err)
	}

	msvc := gmail.NewUsersMessagesService(svc)

	c := controller{
		user: user,
		msvc: msvc,
		mode: mode,
		rate: rate,
	}

	if flag.NArg() > 0 {
		for _, name := range flag.Args() {
			if err := c.handleFolder(ctx, name); err != nil {
				log.Fatal(err)
			}
		}
	} else {
		if err := c.handleMsgContent(os.Stdin); err != nil {
			log.Fatal(err)
		}
	}
}

type controller struct {
	user string
	msvc *gmail.UsersMessagesService
	mode string
	rate time.Duration
}

func (c controller) handleFolder(ctx context.Context, name string) error {
	log.Printf("Opening folder %s", name)

	limiter := rate.NewLimiter(rate.Every(c.rate), 1)

	f, err := folder.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	for {
		msg, err := f.Message()
		if err != nil {
			return err
		}
		if msg == nil {
			return nil
		}
		if err := limiter.Wait(ctx); err != nil {
			return err
		}
		if err := c.handleMsg(msg); err != nil {
			log.Printf("Error handling message (will continue): %s", err)
		}
	}
}

func (c controller) handleMsg(msg io.ReadCloser) error {
	defer msg.Close()

	return c.handleMsgContent(msg)
}

func (c controller) handleMsgContent(r io.Reader) error {
	inp, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "reading message")
	}
	inpMsg := &gmail.Message{
		Raw:      base64.URLEncoding.EncodeToString(inp),
		LabelIds: []string{"INBOX", "UNREAD"},
	}

	var doer doer
	if c.mode == "import" {
		call := c.msvc.Import(c.user, inpMsg)
		call.InternalDateSource("dateHeader")
		doer = call
	} else {
		call := c.msvc.Insert(c.user, inpMsg)
		call.InternalDateSource("dateHeader")
		doer = call
	}

	msg, err := doer.Do()
	if err != nil {
		return errors.Wrapf(err, "in %s", c.mode)
	}

	log.Printf("new message ID %s", msg.Id)
	return nil
}
