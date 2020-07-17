// Command ginsu is a Gmail INSerter for U.
package ginsu

import (
	"context"
	"encoding/base64"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bobg/folder/v3"
	"github.com/bobg/oauther/v3"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func Message(ctx context.Context, gmailSvc *gmail.Service, r io.Reader, isInsert bool) error {
	type doer interface {
		Do(opts ...googleapi.CallOption) (*gmail.Message, error)
	}

	msgBytes, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "reading input")
	}
	inp := &gmail.Message{
		Raw:      base64.URLEncoding.EncodeToString(inp),
		LabelIds: []string{"INBOX", "UNREAD"},
	}

	mSvc := gmailSvc.Users.Messages
	if isInsert {
		call := mSvc.Insert("me", inp)
		call.InternalDateSource("dateHeader")
		doer = call
	} else {
		call := mSvc.Import("me", inp)
		call.InternalDateSource("dateHeader")
		doer = call
	}

	msg, err := doer.Do()
	if err != nil {
		return errors.Wrap(err, "adding message")
	}

	log.Printf("new message ID %s", msg.Id)
	return nil
}

func Folder(ctx context.Context, gmailSvc *gmail.Service, name string, isInsert bool) error {
	f, err := folder.Open(name)
	if err != nil {
		return errors.Wrapf(err, "opening folder %s", name)
	}
	defer f.Close()

	for {
		msg, err := f.Message()
		if err != nil {
			return errors.Wrap(err, "reading message from folder")
		}
		if msg == nil {
			return nil
		}
		err = func() error {
			defer msg.Close()
			return Message(ctx, gmailSvc, msg, isInsert)
		}()
		if err != nil {
			return errors.Wrap(err, "handling message from folder")
		}
	}
}

type doer interface {
	Do(opts ...googleapi.CallOption) (*gmail.Message, error)
}

func main() {
	ctx := context.Background()

	var (
		doImport  = flag.Bool("import", false, "import mode (more scanning)")
		doInsert  = flag.Bool("insert", false, "insert mode (less scanning)")
		user      = flag.String("user", "", "Gmail user ID")
		credsFile = flag.String("creds", "creds.json", "path to credentials file")
		tokenFile = flag.String("token", "token.json", "token cache file")
		code      = flag.String("code", "", "auth code")
		ratestr   = flag.String("rate", "100ms", "rate limit in folder-parsing mode")
	)

	flag.Parse()

	if *doImport && *doInsert {
		log.Fatal("specify only one of -import and -insert")
	}
	if !*doImport && !*doInsert {
		log.Fatal("specify one of -import and -insert")
	}
	if *user == "" {
		log.Fatal("supply a username with -user")
	}

	creds, err := ioutil.ReadFile(*credsFile)
	if err != nil {
		log.Fatal(err)
	}

	oauthClient, err := oauther.Client(ctx, *tokenFile, *code, creds, gmail.GmailInsertScope)
	if c, ok := err.(oauther.ErrNeedAuthCode); ok {
		log.Fatalf("get auth code from %s, then rerun %s -code <code>", c.URL, strings.Join(os.Args, " "))
	}
	if err != nil {
		log.Fatal(err)
	}

	svc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		log.Fatal(err)
	}

	msvc := gmail.NewUsersMessagesService(svc)

	handlemsg := func(r io.Reader) {
		inp, err := ioutil.ReadAll(r)
		if err != nil {
			log.Fatal(err)
		}
		inpMsg := &gmail.Message{
			Raw:      base64.URLEncoding.EncodeToString(inp),
			LabelIds: []string{"INBOX", "UNREAD"},
		}

		var doer doer
		if *doImport {
			call := msvc.Import(*user, inpMsg)
			call.InternalDateSource("dateHeader")
			doer = call
		} else {
			call := msvc.Insert(*user, inpMsg)
			call.InternalDateSource("dateHeader")
			doer = call
		}

		msg, err := doer.Do()
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("new message ID %s", msg.Id)
	}

	if flag.NArg() > 0 {
		dur, err := time.ParseDuration(*ratestr)
		if err != nil {
			log.Fatal(err)
		}
		limiter := rate.NewLimiter(rate.Every(dur), 1)

		for _, name := range flag.Args() {
			log.Printf("opening folder %s", name)
			f, err := folder.Open(name)
			if err != nil {
				log.Fatal(err)
			}
			func() {
				defer f.Close()
				for {
					msg, err := f.Message()
					if err != nil {
						log.Fatal(err)
					}
					if msg == nil {
						break
					}
					func() {
						defer msg.Close()
						err := limiter.Wait(ctx)
						if err != nil {
							log.Fatal(err)
						}
						handlemsg(msg)
					}()
				}
			}()
		}
	} else {
		handlemsg(os.Stdin)
	}
}
