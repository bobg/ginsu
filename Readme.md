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
| $HOME/go/bin/ginsu add -user username@gmail.com
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
version 1.26 or later,
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
ginsu auth
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
You should now have a `token.json` file.
Like `creds.json`,
you should keep the contents of this file secret.

### Command line

Running Ginsu to add a message to your Gmail account looks like this:

```sh
ginsu add -user ADDRESS@gmail.com
```

(where ADDRESS is your Gmail address).
This will use the credentials in `creds.json`
and the token in `token.json`.
It will read an email message from its [standard input](https://en.wikipedia.org/wiki/Standard_streams#Standard_input_%28stdin%29)
(which you can supply by piping it in with `|`
or redirecting from a file with `<`).

If your credentials are in a file named something other than `creds.json`,
you can add `-creds FILENAME` to tell Ginsu where to find them.

If your token is in a file named something other than `token.json`,
you can add `-token FILENAME` to tell Ginsu where to find that.

Finally, if you have one or more folders full of email messages to add to Gmail
(e.g. if you accumulate them on your email server and process them in batches,
rather than one at a time),
you can add the contents of those folders like this:

```sh
ginsu add-from -user ADDRESS@gmail.com FOLDER1 FOLDER2 ...
```

Here is the complete set of subcommands understood by Ginsu,
and their arguments,
with explanations below.

| Command          | Arguments                                                                       | Description                                                                        |
|------------------|---------------------------------------------------------------------------------|------------------------------------------------------------------------------------|
| ginsu add        | -user NAME@gmail.com [-fast] [-creds CREDSFILE] [-token TOKENFILE]              | Add a message from stdin to the NAME@gmail.com account.                            |
| ginsu add-client | -user NAME@gmail.com [-fast] [-privkey PRIVKEYFILE] [-url SERVER]               | Use a Ginsu server to add a message from stdin to the NAME@gmail.com account.      |
| ginsu add-from   | -user NAME@gmail.com [-fast] [-rate RATE] [-creds CREDSFILE] [-token TOKENFILE] | Add messages from folders named on the command line to the NAME@gmail.com account. |
| ginsu auth       | [-creds CREDSFILE] [-token TOKENFILE]                                           | Create or update the Ginsu authentication token.                                   |
| ginsu serve      | [-addr LISTEN] [-dir DIR]                                                       | Start a Ginsu server.                                                              |

The `-fast` flag means to skip the usual processing (such as spam filtering) when adding a message to Gmail.

CREDSFILE defaults to `creds.json`.

TOKENFILE defaults to `token.json`.

PRIVKEYFILE defaults to `privkey`.
It must contain an ED25519 private key.
The matching public key goes into the server config (see below).
You can create an ed25519 private/public keypair like this:

```sh
go install github.com/bobg/txvm/cmd/ed25519@latest
ed25519 gen > privkey
ed25519 pub < privkey > pubkey
```

SERVER is the URL of a Ginsu server “add” endpoint.
It defaults to `http://localhost:7949/add`.

RATE is the rate at which messages should be added from a folder to Gmail,
expressed as a time delay between additions.
The default is 100ms.

LISTEN is the listen address of the server.
It defaults to `localhost:7949`.

DIR is the path of the directory containing the server config.
It defaults to `config.d`.
See details below.

### Running a Ginsu server

TBD
