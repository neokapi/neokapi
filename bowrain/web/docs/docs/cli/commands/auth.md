---
title: auth
sidebar_position: 9
---

# kapi auth

Authenticate the CLI against a Bowrain server, so CLI commands can reach
workspace-scoped resources, and manage the tokens a machine uses in your place.

## Commands

### auth login

Start an interactive login using the OAuth device flow:

```bash
kapi auth login
```

The CLI displays a URL and a one-time code. Open the URL in your browser,
enter the code, and authorize the application. The CLI polls automatically and
stores your token on success.

```
$ kapi auth login
Open https://app.bowrain.cloud/auth/device and enter code: ABCD-1234
Waiting for authorization...
Logged in as reviewer@example.com
```

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
signed in, claiming it transfers the project into your personal workspace on
the server, preserving its files and their results:

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

The login flow uses the [OAuth 2.0 Device Authorization Grant](https://www.rfc-editor.org/rfc/rfc8628)
(RFC 8628), the same flow used by tools like `gh auth login` and `gcloud auth login`.
This works in headless environments (SSH sessions, CI containers) where a browser
redirect is not available.

1. CLI requests a device code from the server
2. User opens the verification URL in any browser and enters the code
3. CLI polls the server until the user authorizes
4. Server issues a JWT token, CLI stores it locally

## Options

| Flag       | Applies to | Description                                             |
| ---------- | ---------- | ------------------------------------------------------- |
| `--server` | `login`    | Server URL to authenticate against; the hosted service when omitted |
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
