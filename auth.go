package main

import (
	"context"
	"net/http"
	"os"

	"github.com/bobg/errors"
	"github.com/bobg/oauther/v5"
	"google.golang.org/api/gmail/v1"
)

func (maincmd) doAuth(ctx context.Context, credsfile, tokenfile string, _ []string) error {
	_, err := authenticate(ctx, credsfile, tokenfile)
	return err
}

func authenticate(ctx context.Context, credsfile, tokenfile string) (*http.Client, error) {
	creds, err := os.ReadFile(credsfile)
	if err != nil {
		return nil, errors.Wrapf(err, "reading credentials file %s", credsfile)
	}
	return oauther.Client(ctx, tokenfile, creds, gmail.GmailInsertScope)
}
