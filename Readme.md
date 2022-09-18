# Ginsu - Gmail INSerter for U

This is ginsu,
a command for inserting email messages directly to a Gmail mailbox.

## Installation

You’ll need to have Go installed, version 1.16 or later. You can get Go at https://go.dev/dl/.

Once you have Go installed, run:

```sh
go install github.com/bobg/ginsu@latest
```

This will install the `ginsu` binary to your `$GOBIN` dir,
normally `$HOME/go/bin`.
Be sure that directory appears in your `$PATH`.

## Usage

You’ll need OAuth credentials in JSON format that allow Ginsu access to your Gmail account.
For information on how to obtain those, see
[Using OAuth 2.0 to Access Google APIs](https://developers.google.com/identity/protocols/oauth2).
Place them in a file named `creds.json`.

Once you have those, you’ll need to use them to populate an OAuth token file. Run this command:

```sh
ginsu -reauth
```

This will show a URL where you must go to get an authorization code.
You will be asked to permit the application called “IMAP fetcher” to have access to your Gmail account.
This is the name of the Google Cloud Platform project to which Ginsu belongs.

You’ll see a warning screen explaining that Google has not verified this application.
Click “Advanced,” then “Go to IMAP fetcher (unsafe),”
if you trust me and this application.
On the next screen you’ll see “IMAP fetcher wants access to your Google Account”
and “When you allow this access, IMAP fetcher will be able to add emails into your Gmail mailbox.”
Click “Continue” to get an authorization code.

(Why is the app not verified?
It costs [beaucoup bucks](https://support.google.com/cloud/answer/9110914?hl=en#sec-assess&zippy=%2Csecurity-assessment) to get it verified,
since [it uses a “restricted scope”](https://github.com/bobg/ginsu/blob/db0051cda1356792ce935fbbf6c114987ecfe843/ginsu.go#L74)
that requires a third-party security verification.)

Once you have the code, enter it where prompted to create the file `token.json`.

Now you may insert email messages to the Inbox in your Gmail account.
To insert a single message, run:

```sh
ginsu -user my.address@gmail.com [-import | -insert]
```

and supply the message on the command’s standard input
(by piping it in with `|` or redirecting from a file with `<`).

You must specify one of `-import` or `-insert` to select the proper mode.
In “import” mode, normal scanning of the incoming message
(for filtering, and to see if it’s spam)
is done as if it were being delivered via SMTP.
In “insert” mode, this scanning is skipped,
as if the message is being added with IMAP.

To insert one or more folders full of email messages, run:

```sh
ginsu -user my.address@gmail.com [-import | -insert] FOLDER ...
```

Other command-line options:

    -creds FILE  Use the named JSON credentials file instead of “creds.json”
    -token FILE  Use the named file for storing the OAuth token instead of “token.json”
    -rate  DUR   When inserting multiple messages, do no more than one insertion each DUR (default: 100ms).
