---
title: auth
sidebar_position: 9
---

# kapi auth

Authenticate the CLI against a Bowrain server, so CLI commands can reach
workspace-scoped resources, and manage the tokens a machine uses in your place.

## Commands

### auth login

Sign in from the terminal:

```bash
kapi auth login
```

The CLI opens your browser at the sign-in page. Once you sign in, the browser
returns you to the terminal and the CLI stores your token.

```
$ kapi auth login
Opening your browser to sign in to https://app.bowrain.cloud.
Waiting for the sign-in to finish in the browser...
Logged in as reviewer@example.com
```

On a machine without a browser of its own, such as an SSH session or a CI
container, the CLI prints a URL and a one-time code instead. Open the URL in a
browser on any device and enter the code. `--use-device-code` asks for this
directly. `--no-browser` prints the sign-in URL without opening it; open it in
a browser on the same machine.

The server is resolved from (first match wins):

1. the `--server` flag
2. `BOWRAIN_SERVER_URL`, or `server.url` in the per-machine
   [bowrain config](/cli/commands/config)
3. the server of the stored login on this machine
4. the hosted service, `https://app.bowrain.cloud`

Self-hosted deployments pass `--server` or set `BOWRAIN_SERVER_URL`:

```bash
kapi auth login --server https://bowrain.example.com
```

The access token is stored in the OS keychain and used automatically by other
CLI commands; see [Token storage](#token-storage).

### auth status

Check the current authentication state:

```bash
kapi auth status
```

Output:

```
Server:  https://app.bowrain.cloud
User:    reviewer@example.com
Name:    Jane Reviewer
Expires: 2026-02-11 14:30:00
```

### auth logout

Remove the stored token:

```bash
kapi auth logout
```

### auth claim

Claim an anonymous project into your workspace. A project created with
`kapi init --anonymous` or `--email` carries a claim token; once you are
signed in, claiming it moves the project into your personal workspace on the
server, with its files and their results:

```bash
kapi auth claim               # uses the claim token stored with the project
kapi auth claim <claim-token> # or the token from the claim link
```

### auth token

Mint, list and revoke API tokens for machines: CI runners, scripts,
integrations. A token acts on the server with the scope it was created with,
and API access is available on every plan.

```bash
kapi auth token create --name "ci" --expire-days 90
kapi auth token list
kapi auth token delete <token-id>
```

The token value (`bwt_…`) is shown once, at creation. On a runner, export it as
`BOWRAIN_AUTH_TOKEN`; the CLI checks that variable before any stored login, and
pairs it with `BOWRAIN_SERVER_URL` on a self-hosted server. See
[The loop in CI](/cli/ci/overview#authenticating-a-runner).

## How it works

On a machine with a browser, login uses the authorization code grant with
PKCE and a loopback redirect ([RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)):

1. The CLI listens on a local port and opens the server's sign-in page with a
   PKCE challenge.
2. You sign in with the identity provider.
3. The server redirects the browser to the local port with a one-time code.
4. The CLI redeems the code with its PKCE verifier, and the server issues the
   tokens in that response.

The redirect carries a one-time code, never a token, and only the CLI that
started the login holds the verifier that redeems it.

Without a browser, login uses the
[device authorization grant](https://www.rfc-editor.org/rfc/rfc8628): the CLI
shows a code, you enter it on any device, and the CLI polls until you approve.

## Options

| Flag       | Applies to | Description                                             |
| ---------- | ---------- | ------------------------------------------------------- |
| `--server` | `login`    | Server URL to authenticate against; the hosted service when omitted |
| `--use-device-code` | `login` | Sign in with a code entered on any device |
| `--no-browser` | `login` | Print the sign-in URL instead of opening the browser |
| `--name`   | `token create` | A label for where the token is used                  |
| `--expire-days` | `token create` | Days until the token expires                    |

## Token storage

The access and refresh tokens live in the OS keychain, under the keys
`bowrain-auth:<server-url>` and `bowrain-refresh:<server-url>`. Only non-secret
metadata is written to `auth.json` in the bowrain config directory
(`~/.config/bowrain` on Linux, `~/Library/Application Support/bowrain` on
macOS; `BOWRAIN_CONFIG_DIR` overrides it):

```json
{
  "server_url": "https://app.bowrain.cloud",
  "expiry": "2026-02-11T14:30:00Z",
  "user": {
    "id": "usr_abc123",
    "email": "reviewer@example.com",
    "name": "Jane Reviewer"
  }
}
```

## Server authentication

Authentication is required when connecting to a `bowrain-server`, which runs as
a multi-user deployment with workspaces. Passkeys and account management live
in the web app's account settings; the CLI signs in through the same identity
provider by way of the device flow.
