// Command ginsu is a Gmail INSerter for U.
// It accepts an e-mail message on standard input
// and uses the Gmail API to "insert" or "import" it.
// Alternatively, it reads the names of mail folders from the command line,
// parses messages out of those,
// and inserts or imports them.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bobg/subcmd/v2"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	var c maincmd

	return subcmd.Run(context.Background(), c, os.Args[1:])
}

type maincmd struct{}

func (c maincmd) Subcmds() subcmd.Map {
	return subcmd.Commands(
		"add", c.doAdd, "add a message to Gmail", subcmd.Params(
			"-fast", subcmd.Bool, false, "use fast insert mode (skips filtering)",
			"-creds", subcmd.String, "creds.json", "path to credentials file",
			"-token", subcmd.String, "token.json", "token cache file",
			"-user", subcmd.String, "", "Gmail user ID",
		),
		"add-client", c.doAddClient, "ask a Ginsu server to add a message to Gmail", subcmd.Params(
			"-fast", subcmd.Bool, false, "use fast insert mode (skips filtering)",
			"-privkey", subcmd.String, "privkey", "path to private ED25519 key file",
			"-url", subcmd.String, "http://localhost:7949/add", "URL of Ginsu server add endpoint",
			"-user", subcmd.String, "", "Gmail user ID",
		),
		"add-from", c.doAddFrom, "add messages from one or more folders to Gmail", subcmd.Params(
			"-fast", subcmd.Bool, false, "use fast insert mode (skips filtering)",
			"-rate", subcmd.Duration, 100*time.Millisecond, "rate limit",
			"-creds", subcmd.String, "creds.json", "path to credentials file",
			"-token", subcmd.String, "token.json", "token cache file",
			"-user", subcmd.String, "", "Gmail user ID",
		),
		"auth", c.doAuth, "authenticate", subcmd.Params(
			"-creds", subcmd.String, "creds.json", "path to credentials file",
			"-token", subcmd.String, "token.json", "token cache file",
		),
		"serve", c.doServe, "run as server", subcmd.Params(
			"-addr", subcmd.String, ":7949", "address to listen on",
			"-dir", subcmd.String, "config.d", "configuration directory",
		),
	)
}
