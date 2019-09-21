// Command ginsu is a Gmail INSerter for U.
// It accepts an e-mail message on standard input and uses the Gmail API to insert it.

package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/bobg/oauther/v2"
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
		doImport  = flag.Bool("import", false, "import mode (more scanning)")
		doInsert  = flag.Bool("insert", false, "insert mode (less scanning)")
		user      = flag.String("user", "", "Gmail user ID")
		credsFile = flag.String("creds", "creds.json", "path to credentials file")
		tokenFile = flag.String("token", "token.json", "token cache file")
		code      = flag.String("code", "", "auth code")
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
	tokSrc := oauther.NewWebTokenSrc(func(url string) (string, error) {
		return "", fmt.Errorf("get an auth code at %s, then rerun this program as %s -code <code>", url, strings.Join(os.Args, " "))
	})
	tokSrc = oauther.NewCodeTokenSrc(tokSrc, *code)
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
	inpMsg := &gmail.Message{
		Raw:      raw,
		LabelIds: []string{"INBOX", "UNREAD"},
	}

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
