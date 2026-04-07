---
title: Troubleshooting
sidebar_position: 5
---

# Troubleshooting

This guide addresses the most common issues encountered when integrating with
the KapiMart platform. If your problem is not listed here, contact our
support team at support@kapimart.com.

## Authentication Errors

### Error: `invalid_api_key`

Your API key was not recognized. Verify the following:

- The key starts with `km_test_` (sandbox) or `km_live_` (production)
- There are no trailing spaces or newline characters
- The key has not been revoked in the Dashboard

### Error: `key_environment_mismatch`

You are using a sandbox key against the production endpoint, or vice versa.
Ensure your `KAPIMART_ENVIRONMENT` setting matches your API key prefix.

## Connection Issues

### Timeout Errors

If requests are timing out, try the following:

1. Increase the timeout value in your configuration
2. Check your network connectivity and firewall rules
3. Verify that the API status page shows no incidents

### SSL Certificate Errors

The SDK requires a valid TLS 1.2+ connection. If you see certificate errors
behind a corporate proxy, configure your HTTP client with the proxy's root
certificate authority.

## Webhook Setup

### Verifying Webhook Signatures

Every webhook request includes an `X-KapiMart-Signature` header. Use the
webhook secret from your Dashboard to verify the signature:

```javascript
import { verifyWebhookSignature } from "@kapimart/webhooks";

const isValid = verifyWebhookSignature(
  requestBody,
  signatureHeader,
  process.env.KAPIMART_WEBHOOK_SECRET
);
```

### Webhook Delivery Failures

If your endpoint returns a non-2xx status code, KapiMart retries up to
5 times with exponential backoff. Check your server logs for errors. Common
causes include:

- Endpoint not publicly reachable
- Request body parsing failures
- Database connection timeouts during event processing

## Migrating from v1

### Breaking Changes in v2

The following changes require code updates when migrating from SDK v1:

- **Monetary amounts** are now in the smallest currency unit (e.g., cents).
  Divide by 100 for display in USD, EUR, and GBP. For zero-decimal currencies
  like JPY, no conversion is needed.
- **Product IDs** changed from sequential integers to prefixed strings
  (e.g., `prod_a1b2c3`).
- **Pagination** moved from offset-based to cursor-based. Replace `offset`
  parameters with the `starting_after` cursor from the previous response.
- **Webhook event names** now use dot notation (`order.created` instead of
  `ORDER_CREATED`).

## Frequently Asked Questions

See the [FAQ](./faq.md) for additional answers to common questions about
billing, supported regions, and platform capabilities.
