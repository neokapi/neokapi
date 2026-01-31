---
sidebar_position: 6
title: Configuration
---

# Configuration

gokapi uses [Viper](https://github.com/spf13/viper) for layered configuration.

## Precedence (highest to lowest)

1. CLI flags (via Cobra)
2. Environment variables (`GOKAPI_*` prefix)
3. Project config (`./gokapi.yaml`)
4. User config (`~/.config/gokapi/gokapi.yaml`)
5. System config (`/etc/gokapi/gokapi.yaml`)
6. Code defaults

## Configuration File

Create a `gokapi.yaml` in your project root:

```yaml
formats:
  html:
    encoding: UTF-8
    preserveWhitespace: false
  xliff:
    version: "2.0"

tools:
  segmentation:
    srxPath: "./rules/default.srx"
  ai-translation:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    apiKey: "${ANTHROPIC_API_KEY}"

plugins:
  directory: "./plugins"
  registry: "https://plugins.gokapi.dev"

flow:
  channelBuffer: 64
  workerPool: 4
```

## Environment Variables

All configuration keys can be set via environment variables with the `GOKAPI_` prefix. Nested keys use underscores:

```bash
# Equivalent to tools.ai-translation.provider in YAML
export GOKAPI_TOOLS_AI_TRANSLATION_PROVIDER=anthropic

# Equivalent to flow.channelBuffer
export GOKAPI_FLOW_CHANNELBUFFER=128
```

## CLI Flags

CLI flags override all other configuration sources:

```bash
kapi translate input.html -o output.html \
  --provider anthropic \
  --model claude-sonnet-4-20250514 \
  -s en -t fr
```

## Config File Locations

gokapi searches for `gokapi.yaml` in these locations:

1. Current directory (`./gokapi.yaml`)
2. User config directory (`~/.config/gokapi/gokapi.yaml`)
3. System config directory (`/etc/gokapi/gokapi.yaml`)

The first file found is used. Values from higher-precedence sources (env vars, CLI flags) override individual keys.
