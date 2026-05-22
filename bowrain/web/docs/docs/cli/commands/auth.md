---
title: auth
sidebar_position: 9
---

# kapi auth

Authenticate the CLI against a deployed `bowrain-server` instance. This enables
CLI commands to access workspace-scoped resources on a remote server.

## Commands

### auth login

Start an interactive login using the OAuth device flow:

```bash
kapi auth login --server https://neokapi.example.com
```

The CLI will display a URL and a one-time code. Open the URL in your browser,
enter the code, and authorize the application. The CLI polls automatically and
stores your token on success.

```
$ kapi auth login --server https://neokapi.example.com
Open https://neokapi.example.com/auth/device and enter code: ABCD-1234
Waiting for authorization...
Logged in as translator@example.com
```

The access token is stored at `~/.config/bowrain/auth.json` and used
automatically by other CLI commands.

### auth status

Check the current authentication state:

```bash
kapi auth status
```

Output:

```
Server:  https://neokapi.example.com
User:    translator@example.com
Name:    Jane Translator
Expires: 2026-02-11 14:30:00
```

### auth logout

Remove the stored token:

```bash
kapi auth logout
```

## How It Works

The login flow uses the [OAuth 2.0 Device Authorization Grant](https://www.rfc-editor.org/rfc/rfc8628)
(RFC 8628), the same flow used by tools like `gh auth login` and `gcloud auth login`.
This works in headless environments (SSH sessions, CI containers) where a browser
redirect is not available.

1. CLI requests a device code from the server
2. User opens the verification URL in any browser and enters the code
3. CLI polls the server until the user authorizes
4. Server issues a JWT token, CLI stores it locally

## Options

| Flag       | Description                                             |
| ---------- | ------------------------------------------------------- |
| `--server` | Server URL to authenticate against (required for login) |

## Token Storage

Tokens are stored in `~/.config/bowrain/auth.json`:

```json
{
  "server_url": "https://neokapi.example.com",
  "access_token": "eyJ...",
  "refresh_token": "...",
  "expiry": "2026-02-11T14:30:00Z",
  "user": {
    "id": "usr_abc123",
    "email": "translator@example.com",
    "name": "Jane Translator"
  }
}
```

## Server Modes

Authentication is only required when connecting to a `bowrain-server` running in
multi-user mode. When using `kapi serve` for local project editing, no
authentication is needed — the local server runs on localhost without auth.

| Mode             | Auth Required | Description                           |
| ---------------- | ------------- | ------------------------------------- |
| `kapi serve`  | No            | Local project server, localhost only  |
| `bowrain-server` | Yes           | Multi-user deployment with workspaces |
