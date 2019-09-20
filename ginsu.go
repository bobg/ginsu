// Command ginsu is a Gmail INSerter for U.
// It accepts an e-mail message on standard input and uses the Gmail API to insert it.

package main

import (
	"context"
	"encoding/base64"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/bobg/oauther/v2"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type doer interface {
	Do(opts ...googleapi.CallOption) (*gmail.Message, error)
}

func main() {
	ctx := context.Background()

	var (
		user      = flag.String("user", "", "Gmail user ID")
		doImport  = flag.Bool("import", false, "import mode (more scanning)")
		doInsert  = flag.Bool("insert", false, "insert mode (less scanning)")
		credsFile = flag.String("creds", "creds.json", "path to credentials file")
		tokenFile = flag.String("tokfile", "token.json", "token cache file")
		token     = flag.String("tok", "", "oauth web token")
	)

	flag.Parse()

	if *doImport && *doInsert {
		log.Fatal("specify only one of -import and -insert")
	}
	if *doImport && *doInsert {
		log.Fatal("specify one of -import and -insert")
	}
	if *user == "" {
		log.Fatal("supply a username with -user")
	}

	creds, err := ioutil.ReadFile(*credsFile)
	if err != nil {
		log.Fatal(err)
	}
	tokSrc := oauther.NewWebTokenSrc(func(url string) (string, error) {
		log.Printf("get a token at %s", url)
		log.Printf("then rerun this program as %s -tok <the token>", strings.Join(os.Args, " "))
		os.Exit(0)
		return "", nil
	})
	tokSrc = valTokSrc{token: *token, src: tokSrc}
	tokSrc = oauther.NewFileCache(tokSrc, *tokenFile)
	oauthClient, err := oauther.HTTPClient(ctx, creds, tokSrc, gmail.GmailInsertScope)
	if err != nil {
		log.Fatal(err)
	}

	svc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		log.Fatal(err)
	}

	msvc := gmail.NewUsersMessagesService(svc)

	inp, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	raw := base64.URLEncoding.EncodeToString(inp)

	var doer doer

	if *doImport {
		call := msvc.Import(*user, &gmail.Message{Raw: raw})
		call.InternalDateSource("dateHeader")
		doer = call
	} else {
		call := msvc.Insert(*user, &gmail.Message{Raw: raw})
		call.InternalDateSource("dateHeader")
		doer = call
	}

	msg, err := doer.Do()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("new message ID %s", msg.Id)
}

type valTokSrc struct {
	token string
	src   oauther.TokenSrc
}

func (v valTokSrc) Get(ctx context.Context, conf *oauth2.Config) (*oauth2.Token, error) {
	if v.token != "" {
		return conf.Exchange(ctx, v.token)
	}
	return v.src.Get(ctx, conf)
}
