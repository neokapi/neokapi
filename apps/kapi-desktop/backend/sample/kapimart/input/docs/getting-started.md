---
title: Getting Started
sidebar_position: 1
---

# Getting Started with KapiMart

Welcome to the KapiMart Developer Platform! This guide will walk you through
setting up your first integration in under 10 minutes.

## Prerequisites

Before you begin, make sure you have the following:

- A KapiMart merchant account (sign up at [merchant.kapimart.com](https://merchant.kapimart.com))
- An API key from your **Dashboard > Settings > API Keys** page
- A development environment with Node.js 18+ or Python 3.10+

## Quick Start

### Step 1: Install the SDK

Choose your preferred language:

```bash
# Node.js
npm install @kapimart/sdk

# Python
pip install kapimart-sdk
```

### Step 2: Initialize the Client

```javascript
import { KapiMart } from "@kapimart/sdk";

const client = new KapiMart({
  apiKey: "km_test_your_api_key_here",
  environment: "sandbox",
});
```

### Step 3: Create Your First Product

```javascript
const product = await client.products.create({
  name: "Wireless Headphones",
  price: 7999, // Amount in cents
  currency: "USD",
  description: "Premium noise-cancelling headphones with 30-hour battery life.",
});

console.log(`Product created: ${product.id}`);
```

## What's Next?

Now that you have the basics working, explore these topics:

- **[Installation Guide](./installation.md)** for detailed setup instructions
- **[Configuration](./configuration.md)** for customizing your integration
- **[API Reference](./api-reference.md)** for the complete endpoint documentation

If you run into any issues, check our [Troubleshooting](./troubleshooting.md)
guide or reach out to our support team at support@kapimart.com.
