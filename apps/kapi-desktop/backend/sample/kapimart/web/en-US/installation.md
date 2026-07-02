---
title: Installation
sidebar_position: 2
---

# Installation Guide

This page covers the complete installation process for the KapiMart SDK,
including system requirements, supported platforms, and optional dependencies.

## System Requirements

| Component       | Minimum Version  | Recommended     |
|-----------------|------------------|-----------------|
| Node.js         | 18.0             | 20.x LTS       |
| Python          | 3.10             | 3.12+           |
| Memory          | 256 MB           | 512 MB          |
| Disk Space      | 50 MB            | 100 MB          |

## Installing the SDK

### Node.js

Install the package using your preferred package manager:

```bash
npm install @kapimart/sdk
# or
yarn add @kapimart/sdk
# or
pnpm add @kapimart/sdk
```

The SDK includes TypeScript type definitions out of the box. No additional
`@types` package is needed.

### Python

Install from PyPI:

```bash
pip install kapimart-sdk
```

For production deployments, we recommend pinning to a specific version:

```bash
pip install kapimart-sdk==2.4.1
```

## Optional Dependencies

### Webhook Verification

To verify incoming webhook signatures, install the crypto extension:

```bash
npm install @kapimart/webhooks
```

### File Upload Support

For product image uploads and document attachments, the file handling
module is required:

```bash
npm install @kapimart/files
```

## Verifying Your Installation

Run this command to check that the SDK is installed correctly:

```bash
npx kapimart-check
```

You should see output like:

```
KapiMart SDK v2.4.1
  Environment: sandbox
  API endpoint: https://api.sandbox.kapimart.com/v2
  Status: OK
```

## Upgrading from v1

If you are upgrading from SDK version 1.x, please review the
[migration guide](./troubleshooting.md#migrating-from-v1) for breaking changes.
The most significant change is that all monetary amounts are now expressed in
the smallest currency unit (cents for USD, yen for JPY).
