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
	"strings"
	"time"

	"github.com/bobg/folder/v3"
	"github.com/bobg/oauther/v3"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
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
		code      = flag.String("code", "", "auth code")
		credsFile = flag.String("creds", "creds.json", "path to credentials file")
		doImport  = flag.Bool("import", false, "import mode (more scanning)")
		doInsert  = flag.Bool("insert", false, "insert mode (less scanning)")
		ratestr   = flag.String("rate", "100ms", "rate limit in folder-parsing mode")
		reauth    = flag.Bool("reauth", false, "reauth")
		tokenFile = flag.String("token", "token.json", "token cache file")
		user      = flag.String("user", "", "Gmail user ID")
	)

	flag.Parse()

	creds, err := os.ReadFile(*credsFile)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	if *reauth {
		err = doReauth(ctx, creds, *tokenFile)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	if *doImport && *doInsert {
		log.Fatal("specify only one of -import and -insert")
	}
	if !*doImport && !*doInsert {
		log.Fatal("specify one of -import and -insert")
	}
	if *user == "" {
		log.Fatal("supply a username with -user")
	}

	oauthClient, err := oauther.Client(ctx, *tokenFile, *code, creds, gmail.GmailInsertScope)
	var cerr oauther.ErrNeedAuthCode
	if errors.As(err, &cerr) {
		log.Fatalf("Get auth code from %s, then rerun %s -code <code>", cerr.URL, strings.Join(os.Args, " "))
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
		inp, err := io.ReadAll(r)
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

func doReauth(ctx context.Context, creds []byte, tokenFile string) error {
	conf, err := google.ConfigFromJSON(creds, gmail.GmailInsertScope)
	if err != nil {
		return errors.Wrap(err, "getting config from creds")
	}

	fmt.Printf("Go to: %s\nand enter the auth code you get: ", conf.AuthCodeURL("state-token", oauth2.AccessTypeOffline))
	var code string
	n, err := fmt.Scanln(&code)
	if err != nil {
		return errors.Wrap(err, "reading code from stdin")
	}
	if n != 1 {
		return fmt.Errorf("read %d values from stdin, want 1", n)
	}
	_, err = oauther.Token(ctx, tokenFile, code, creds, gmail.GmailInsertScope)
	return errors.Wrap(err, "getting OAuth token")
}
