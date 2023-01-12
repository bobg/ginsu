# Ginsu - Gmail INSerter for U

This is ginsu,
a command for inserting email messages directly to a Gmail mailbox.

## The problem this solves

If you operate your own email server,
but would like incoming mail to be copied to your Gmail account,
you might naively expect that you can use email forwarding to achieve this.

But as you will soon discover,
Google will sometimes throttle your forwarded mail,
so that messages do not show up in Gmail until many minutes later than they should.
You may also occasionally encounter the dreaded [SMTP bounce loop](https://en.wikipedia.org/wiki/Email_loop).

Both problems can be avoided by using Gmail’s API to add messages to your Gmail account,
rather than conventional forwarding.
This is what Ginsu helps you to do.

## How it works

When the `ginsu` program runs,
it reads an email message as input
and adds it to a specified Gmail account.

You must arrange to run `ginsu` for each incoming message,
ideally as it arrives on your email server.

One good way to do this is with a “message delivery agent” like [procmail](https://en.wikipedia.org/wiki/Procmail).
The behavior of `procmail` is controlled by a file called `.procmailrc`.
Invoking `ginsu` from a `.procmailrc` file looks like this:

```
:0 c
| $HOME/go/bin/ginsu -user username@gmail.com -mode import
```

Here, `:0` introduces a `procmail` mail-processing rule,
and `c` means “operate on a copy of the message”
(so that after this copy is sent to Ginsu,
processing will continue with another copy,
e.g. to place it in a local mail folder).

See [Usage](#usage) below for an explanation of the `ginsu` command and its arguments.

## Installation

Ginsu is written in the Go language,
and is installed with the `go` command.

If you don’t already have `go` installed,
version 1.16 or later,
you can get it at [https://go.dev/dl/](https://go.dev/dl/).

Once Go is installed, run:

```sh
go install github.com/bobg/ginsu@latest
```

This will install the `ginsu` program to your `$GOBIN` dir,
which is normally `$GOPATH/bin`,
which is normally `$HOME/go/bin`.

## Usage

To use `ginsu`,
you must grant it permission to access your Gmail account.
This is done with two special files:
a “credentials” file and a “token” file.

The credentials file identifies the Ginsu program to Google.
The token file tells Google that you’ve permitted Ginsu to access your account.

### The credentials file

To get a credentials file,
you’ll request one from the Google Cloud console.

Go to [console.cloud.google.com](https://console.cloud.google.com)
and create a “project”
(or select a suitable one if you already have some defined).
Then use the menu to navigate to `APIs & Services`
and from there to `Credentials`.

Choose `Create Credentials`
and then choose credential type `OAuth Client ID`.
For “Application type” choose `Desktop app`.

Once your credentials are created,
download them in JSON format to the filename `creds.json`.

Keep the contents of this file secret.

### The token file

Once you have the `creds.json` file,
you can get the token that authorizes Ginsu to access your Gmail account.

Run:

```sh
ginsu -mode auth
```

(If the `ginsu` command is not found,
make sure the directory where you installed it appears in your [PATH](https://en.wikipedia.org/wiki/PATH_%28variable%29).)

This will open a page in your web browser where you’ll be asked to log into your Google account.
Once you do,
you’ll see a “Google hasn’t verified this app” warning.
Assuming you trust Ginsu,
you can proceed by clicking `Advanced`, and then the `Go to PROJECT (unsafe)` link
(where PROJECT is the name of the Google Cloud project
associated with the credentials you created).

The next screen tells you that “PROJECT wants access to your Google account.”
Clicking `Continue` finishes the authorization process.
You should now have a `ginsu-token.json` file.
Like `ginsu-creds.json`,
you should keep the contents of this file secret.

### Command line

Running Ginsu to add a message to your Gmail account looks like this:

```sh
ginsu -user ADDRESS@gmail.com -mode import
```

(where ADDRESS is your Gmail address).
This will use the credentials in `creds.json`
and the token in `token.json`.
It will read an email message from its [standard input](https://en.wikipedia.org/wiki/Standard_streams#Standard_input_%28stdin%29)
(which you can supply by piping it in with `|`
or redirecting from a file with `<`).

The `-mode import` arguments tell Ginsu to operate in “import” mode,
in which Gmail does its normal incoming-message processing:
applying user-defined filters, and checking for spam.
You could choose instead to use `-mode insert`,
which skips this processing.

If your credentials are in a file named something other than `creds.json`,
you can add `-creds FILENAME` to tell Ginsu where to find them.

If your token is in a file named something other than `token.json`,
you can add `-token FILENAME` to tell Ginsu where to find that.

Finally, if you have one or more folders full of email messages to add to Gmail
(e.g. if you accumulate them on your email server and process them in batches,
rather than one at a time),
you can add the folder names to the command line,
like this:

```sh
ginsu -user ADDRESS@gmail.com -mode import FOLDER1 FOLDER2 ...
```

In this case,
Ginsu will read messages from those files
rather than a single message from standard input.
