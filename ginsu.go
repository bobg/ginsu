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
	"github.com/bobg/oauther/v4"
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
		user, ratestr        string
		mode                 string // "import", "insert", "auth"
	)
	flag.StringVar(&credsfile, "creds", "creds.json", "path to credentials file")
	flag.StringVar(&mode, "mode", "", "mode (import, insert, or auth)")
	flag.StringVar(&ratestr, "rate", "100ms", "rate limit in folder-parsing mode")
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

	if user == "" {
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
		if mode == "import" {
			call := msvc.Import(user, inpMsg)
			call.InternalDateSource("dateHeader")
			doer = call
		} else {
			call := msvc.Insert(user, inpMsg)
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
		dur, err := time.ParseDuration(ratestr)
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
