---
title: Release Notes
sidebar_position: 7
---

# Release Notes

## Version 2.4.1 — April 2025

### Bug Fixes

- Fixed an issue where webhook retries could be sent to the wrong endpoint
  after an endpoint URL change.
- Resolved a currency formatting error for Norwegian krone (NOK) amounts
  in the checkout flow.
- Corrected the `total_weight` calculation for orders containing bundled
  products.

## Version 2.4.0 — March 2025

### New Features

- **Multi-warehouse fulfillment:** Orders can now be split across multiple
  warehouses automatically based on inventory availability and shipping cost.
- **Arabic language support:** The storefront checkout and email templates
  now support right-to-left (RTL) rendering for Arabic locales.
- **Bulk product import:** Import up to 10,000 products at once via CSV
  upload in the Dashboard.

### Improvements

- Reduced API response times by 40% for product listing queries through
  improved database indexing.
- Added `last_ordered_at` field to the Product response for better
  inventory planning.
- Webhook delivery now includes a `X-KapiMart-Delivery-ID` header for
  deduplication.

## Version 2.3.0 — January 2025

### New Features

- **Gift cards:** Create, sell, and redeem digital gift cards. Supports
  custom amounts and expiration dates.
- **Customer groups:** Segment customers into groups for targeted pricing,
  promotions, and access control.

### Improvements

- Shipping cost estimation now accounts for dimensional weight in addition
  to actual weight.
- The order confirmation email template includes a direct link to the
  tracking page.

### Deprecations

- The `GET /products?offset=N` pagination style is deprecated. Please
  migrate to cursor-based pagination using `starting_after`. The offset
  parameter will be removed in version 3.0.

## Version 2.2.0 — November 2024

### New Features

- **Discount codes:** Create percentage-based and fixed-amount discount codes
  with usage limits and expiration dates.
- **Inventory alerts:** Receive notifications when product stock falls below
  a configurable threshold.

### Bug Fixes

- Fixed duplicate webhook deliveries that occurred when the API returned a
  timeout but the request had already been processed.
- Resolved an encoding issue with Japanese product names in exported CSV
  files.
