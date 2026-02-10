---
sidebar_position: 2
title: translate
---

# kapi translate

Translate a document using AI or a machine translation service.

## Synopsis

```bash
kapi translate <input> -o <output> -s <source> -t <target> [flags]
```

## Description

The `translate` command reads a document, translates all translatable content, and writes the translated document. It can use AI providers (Anthropic, OpenAI, Ollama) or machine translation services (DeepL, Google, Microsoft, ModernMT, MyMemory).

## Examples

```bash
# Translate with AI (uses configured provider)
kapi translate input.html -o output.html -s en -t fr

# Translate with a specific provider
kapi translate input.json -o output.json -s en -t de --provider anthropic

# Translate with DeepL
kapi translate input.html -o output.html -s en -t ja --mt deepl

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
| `--mt` | | Machine translation service (deepl, google, microsoft, modernmt, mymemory) |
| `--tm` | | Translation memory file (TMX) |
| `--glossary` | | Glossary file for terminology constraints |
