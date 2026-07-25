---
title: FAQ
sidebar_position: 6
---

# Frequently Asked Questions

## General

### What is KapiMart?

KapiMart is a cloud-based e-commerce platform that enables businesses to build
and manage online stores. The platform provides APIs for product management,
order processing, payment handling, and customer engagement.

### Which countries does KapiMart support?

KapiMart is available in over 35 countries across North America, Europe, and
Asia-Pacific. We support 42 currencies and offer localized checkout experiences
in 28 languages. See our [coverage page](https://kapimart.com/coverage) for
the full list.

### Is there a free trial?

Yes! Every new account starts with a 14-day free trial of the Professional
plan. No credit card is required. After the trial, you can continue with the
free Starter plan or upgrade to unlock advanced features.

## Pricing and Billing

### How is pricing calculated?

Pricing is based on your monthly transaction volume. The Starter plan is free
for up to 100 orders per month. The Professional plan starts at $49/month and
includes up to 5,000 orders. Enterprise customers receive custom pricing.

### Can I change my plan at any time?

Yes, you can upgrade or downgrade your plan at any time from the Dashboard.
When upgrading, the new features are available immediately. When downgrading,
the change takes effect at the start of the next billing cycle.

### What payment methods do you accept?

We accept Visa, Mastercard, American Express, and bank transfers (SEPA and
ACH). Annual plans are eligible for a 15% discount when paid upfront.

## Technical

### What is the API rate limit?

Sandbox accounts are limited to 100 requests per second. Production accounts
can make up to 500 requests per second. If you need higher limits, contact
our sales team to discuss Enterprise options.

### Do you support GraphQL?

Not at this time. Our REST API provides complete coverage of all platform
features. We are evaluating GraphQL for a future release based on customer
demand.

### How do I handle multi-currency pricing?

Set the `currency` field when creating products or use the locale-based
pricing feature to define different prices for each market. The API
automatically converts and formats prices based on the customer's locale.

### Can I use KapiMart with my existing website?

Absolutely. KapiMart offers a headless commerce API, embeddable widgets,
and pre-built components for React, Vue, and Angular. You can integrate the
checkout flow into any website or mobile application.

## Security

### Is my data encrypted?

All data is encrypted in transit using TLS 1.3 and at rest using AES-256.
Payment card data is processed through PCI DSS Level 1 certified
infrastructure and is never stored on our servers.

### How do you handle GDPR compliance?

KapiMart provides tools for data export, deletion requests, and consent
management. Our Data Processing Agreement (DPA) is available for download
in the Dashboard under **Settings > Legal**.
