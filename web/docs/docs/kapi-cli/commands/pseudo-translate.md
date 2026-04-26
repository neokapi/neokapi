---
sidebar_position: 5
title: pseudo-translate
---

# kapi pseudo-translate

Generate pseudo-translations for localization testing. Replaces characters with accented variants and adds padding to catch truncation bugs and layout issues.

## Synopsis

```bash
kapi pseudo-translate [files...] [flags]
```

## Aliases

`kapi pseudo`

## Description

Pseudo-translation transforms source text into an accented, padded variant that remains readable but exposes localization issues:

- **Character replacement**: ASCII → accented equivalents (e.g., `a` → `ä`, `e` → `ë`)
- **Text expansion**: Adds padding to simulate longer translations (configurable)
- **Bracket wrapping**: `[!!! Hëëëröööö !!!]` makes untranslated strings visible

## Examples

```bash
# Pseudo-translate a JSON file
kapi pseudo-translate messages.json --target-lang qps

# Specify output path
kapi pseudo-translate messages.json --target-lang fr -o messages-pseudo.json

# Use output path template
kapi pseudo-translate src/locales/en/*.json --target-lang qps -o "src/locales/qps/{name}{ext}"

# Add 30% text expansion
kapi pseudo-translate messages.json --target-lang qps --expansion 30

# Process multiple files in parallel
kapi pseudo-translate src/**/*.json --target-lang qps -j 4

# Override format detection
kapi pseudo-translate data.txt --format json --target-lang qps
```

## Flags

| Flag                | Short | Description                                              | Default |
| ------------------- | ----- | -------------------------------------------------------- | ------- |
| `--target-lang`     |       | Target language code (BCP 47)                            |         |
| `--source-lang`     |       | Source language code                                     | `en`    |
| `--output`          | `-o`  | Output path template (`\{name\}`, `\{ext\}`, `\{lang\}`) |         |
| `--expansion`       |       | Text expansion percentage (0 = none)                     | `0`     |
| `--format`          | `-f`  | Override input format detection                          |         |
| `--encoding`        | `-e`  | Input file encoding                                      | `UTF-8` |
| `--concurrency`     | `-j`  | Max parallel files (0 = auto)                            | `0`     |
| `--progress`        | `-p`  | Show progress bar                                        | `false` |
| `--fail-on-unknown` |       | Fail on unrecognized formats (default: skip)             | `false` |
| `--no-warn`         |       | Suppress warnings for skipped files                      | `false` |

## Output Path Templates

| Variable   | Description                  | Example    |
| ---------- | ---------------------------- | ---------- |
| `\{name\}` | Filename without extension   | `messages` |
| `\{ext\}`  | File extension including dot | `.json`    |
| `\{lang\}` | Target language code         | `qps`      |

## Use Cases

- **UI testing**: Verify that layouts handle longer text
- **RTL testing**: Check right-to-left language support
- **Missing translations**: Spot untranslated strings in the UI
- **CI/CD**: Automated pseudo-translation builds for visual QA
