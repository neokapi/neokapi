---
sidebar_position: 3
---

# Installation

Follow these steps to install Acme on your system.

## Using npm

```bash
npm install -g @acme/cli
```

## Using Homebrew (macOS)

```bash
brew install acme
```

## Using Docker

```bash
docker pull acme/cli:latest
docker run --rm acme/cli version
```

## Verify Installation

After installation, verify that the CLI is working:

```bash
acme --version
```

You should see the version number printed to the console.

## Authentication

Sign in to your Acme account:

```bash
acme login
```

This will open your browser for authentication. Once complete, you can start using the CLI.

## System Requirements

| Requirement | Minimum | Recommended |
|---|---|---|
| Operating System | macOS 12, Ubuntu 20.04, Windows 10 | Latest stable |
| Node.js | 18.x | 20.x or later |
| Memory | 512 MB | 2 GB |
| Disk Space | 200 MB | 1 GB |

## Troubleshooting

If you encounter issues during installation, try the following:

1. Clear your npm cache: `npm cache clean --force`
2. Check your Node.js version: `node --version`
3. Visit our [community forum](https://community.acme.example) for help
