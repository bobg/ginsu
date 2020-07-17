package ginsu

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bobg/aesite"
	"github.com/bobg/mid"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func (s *Server) handleHome(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	var workingToken bool

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if aesite.IsNoSession(err) {
		// ok
	} else if err != nil {
		return errors.Wrap(err, "getting session")
	} else {
		var u user
		err = sess.GetUser(ctx, s.dsClient, &u)
		if stderrs.Is(err, aesite.ErrAnonymous) {
			// ok
		} else if err != nil {
			return errors.Wrap(err, "getting session user")
		}

		if u.Token != "" {
			var token oauth2.Token
			err = json.Unmarshal([]byte(u.Token), &token)
			if err != nil {
				return errors.Wrap(err, "JSON-unmarshaling token")
			}

			client := oauthConf.Client(ctx, &token)
			gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
			if err != nil {
				// xxx
			}
			prof, err := gmailSvc.Users.GetProfile("me").Do()
			if err != nil {
				// xxx
			}

			workingToken = true
		}
	}

	if !workingToken {
		url := conf.AuthCodeURL(csrf, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
		http.Redirect(w, req, url, http.StatusSeeOther)
		return nil
	}

	// xxx render "working token" page
}

func (s *Server) handleMessage(w http.ResponseWriter, req *http.Request) error {
	// xxx check that method is POST
	var (
		ctx    = req.Context()
		email  = req.FormValue("email")
		key    = req.FormValue("key")
		insert = req.FormValue("insert")
		u      user
		err    error
	)

	var isInsert bool
	if insert != "" {
		isInsert, err = strconv.ParseBool(insert)
		if err != nil {
			return errors.Wrapf(err, "parsing insert=%s", insert)
		}
	}

	err = aesite.LookupUser(ctx, s.dsClient, email, &u)
	if err != nil {
		return errors.Wrapf(err, "looking up user %s", email)
	}
	if key != u.InsertionKey {
		return mid.CodeErr{C: http.StatusUnauthorized}
	}

	client, err := s.getOauthClient(ctx, &u)
	if err != nil {
		return errors.Wrap(err, "getting oauth client")
	}

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return errors.Wrap(err, "getting gmail client")
	}

	err = Message(ctx, gmailSvc, req.Body, isInsert)
	return errors.Wrap(err, "adding message")
}
