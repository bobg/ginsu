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

	"github.com/bobg/oauther/v3"
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

	inp, err := ioutil.ReadAll(os.Stdin)
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
