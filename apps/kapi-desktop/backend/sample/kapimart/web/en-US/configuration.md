---
title: Configuration
sidebar_position: 3
---

# Configuration

The KapiMart SDK can be configured through environment variables, a
configuration file, or directly in code. Configuration options are evaluated
in the following order of precedence:

1. Constructor arguments (highest)
2. Environment variables
3. Configuration file (`kapimart.config.yaml`)
4. Default values (lowest)

## Environment Variables

| Variable                  | Description                          | Default            |
|---------------------------|--------------------------------------|--------------------|
| `KAPIMART_API_KEY`        | Your API key                         | _(required)_       |
| `KAPIMART_ENVIRONMENT`    | `sandbox` or `production`            | `sandbox`          |
| `KAPIMART_TIMEOUT`        | Request timeout in milliseconds      | `30000`            |
| `KAPIMART_MAX_RETRIES`    | Number of automatic retries          | `3`                |
| `KAPIMART_LOG_LEVEL`      | Logging verbosity: debug, info, warn | `info`             |
| `KAPIMART_WEBHOOK_SECRET` | Secret for webhook signature checks  | _(optional)_       |

## Configuration File

Create a `kapimart.config.yaml` file in your project root:

```yaml
api_key: km_test_your_api_key_here
environment: sandbox
timeout: 30000
max_retries: 3
log_level: info

# Regional settings
region: eu-west
currency: EUR
locale: de-DE

# Rate limiting
rate_limit:
  max_requests_per_second: 10
  burst_size: 25

# Webhook configuration
webhooks:
  secret: whsec_your_webhook_secret
  tolerance: 300 # seconds
```

## Locale and Currency Settings

KapiMart supports multi-currency and multi-locale storefronts. Set the default
locale and currency for your integration:

```javascript
const client = new KapiMart({
  apiKey: process.env.KAPIMART_API_KEY,
  locale: "ja-JP",
  currency: "JPY",
  // Prices returned in yen (no decimal subdivision)
});
```

Supported locales include all BCP-47 language tags. The platform currently
supports 42 currencies. See the [API reference](./api-reference.md) for
the full list.

## Proxy Configuration

For environments behind a corporate proxy, configure the HTTP client:

```javascript
const client = new KapiMart({
  apiKey: process.env.KAPIMART_API_KEY,
  httpAgent: new HttpsProxyAgent("http://proxy.corp.example.com:8080"),
});
```

## Next Steps

- Review the [API Reference](./api-reference.md) for endpoint details
- Set up [webhooks](./troubleshooting.md#webhook-setup) for real-time events
