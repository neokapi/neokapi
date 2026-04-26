---
sidebar_position: 7
title: word-count
---

# kapi word-count

Count words in source and target text for translation cost estimation.

## Synopsis

```bash
kapi word-count [files...] [flags]
```

## Aliases

`kapi wc`

## Description

Counts words and characters in translatable content, useful for estimating translation costs and project scope. Supports all formats that kapi can read.

## Examples

```bash
# Count words in a single file
kapi word-count messages.json

# Count words in multiple files
kapi word-count src/locales/en/*.json

# Specify source and target languages
kapi word-count messages.json --source-lang en --target-lang fr

# JSON output for CI/CD
kapi word-count messages.json --json

# Override format detection
kapi word-count data.txt --format json

# Process files in parallel
kapi word-count src/**/*.json -j 4
```

## Flags

| Flag                | Short | Description                                  | Default |
| ------------------- | ----- | -------------------------------------------- | ------- |
| `--source-lang`     |       | Source language code                         | `en`    |
| `--target-lang`     |       | Target language code                         |         |
| `--format`          | `-f`  | Override input format detection              |         |
| `--encoding`        | `-e`  | Input file encoding                          | `UTF-8` |
| `--concurrency`     | `-j`  | Max parallel files (0 = auto)                | `0`     |
| `--progress`        | `-p`  | Show progress bar                            | `false` |
| `--fail-on-unknown` |       | Fail on unrecognized formats (default: skip) | `false` |
| `--no-warn`         |       | Suppress warnings for skipped files          | `false` |
| `--json`            |       | Output results as JSON                       | `false` |

## Use Cases

- **Cost estimation**: Calculate translation costs based on word count
- **Project scoping**: Understand the size of translation projects
- **Progress tracking**: Monitor how much content has been translated
- **CI/CD reporting**: Automated word count in build pipelines
