---
sidebar_position: 2
title: translate
---

# kapi translate

Translate a document using AI or a translation connector.

## Synopsis

```bash
kapi translate <input> -o <output> -s <source> -t <target> [flags]
```

## Description

The `translate` command reads a document, translates all translatable content, and writes the translated document. It can use AI providers (Anthropic, OpenAI, Ollama) or translation connectors (DeepL, Google, Microsoft).

## Examples

```bash
# Translate with AI (uses configured provider)
kapi translate input.html -o output.html -s en -t fr

# Translate with a specific provider
kapi translate input.json -o output.json -s en -t de --provider anthropic

# Translate with DeepL connector
kapi translate input.html -o output.html -s en -t ja --connector deepl

# Translate with TM leverage first
kapi translate input.html -o output.html -s en -t fr --tm project.tmx
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output file path (required) |
| `--source-lang` | `-s` | Source language (required) |
| `--target-lang` | `-t` | Target language (required) |
| `--provider` | | AI provider (anthropic, openai, ollama) |
| `--model` | | Model name |
| `--connector` | | Translation connector (deepl, google, microsoft) |
| `--tm` | | Translation memory file (TMX) |
| `--glossary` | | Glossary file for terminology constraints |
