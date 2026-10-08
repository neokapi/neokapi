# Networking

## Custom domains

Add a domain with `harbor domains add <app> <domain>`, then create the CNAME
record Harbor prints. Harbor requests a TLS certificate once the record
resolves.

## IP allowlist

Limit who can reach an app by listing the addresses it accepts:

```toml
[network]
allow = ["203.0.113.0/24"]
```

Requests from any other address get a `403` response. Leave `allow` empty to
accept traffic from anywhere.
