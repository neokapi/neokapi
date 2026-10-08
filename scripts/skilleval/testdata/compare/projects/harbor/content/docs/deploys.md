# Deploys

Run `harbor deploy` from the directory that holds `harbor.toml`. Harbor builds
an image, starts the new release beside the current one, and moves traffic once
the new release passes its health check.

## Health checks

Harbor calls the path in `[health].path` every 5 seconds. A release passes
when 3 calls in a row return a `2xx` status. If it does not pass within
`[health].timeout` (default 120 seconds), Harbor stops the release and keeps
traffic on the current one.

```toml
[health]
path = "/healthz"
timeout = 120
```

## Watch a deploy

Run `harbor logs --release <release-id>` to follow the new release's output
while it starts.
