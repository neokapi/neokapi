# ADR-008: Viper-based layered configuration

## Context

The framework needs configuration at multiple levels: global defaults, user
preferences, project settings, per-format options, per-tool options, and CLI
flags. Okapi uses XML-based properties files which are verbose and unfriendly
for modern workflows. Configuration must also work in CI/CD environments
(environment variables) and team settings (committed project files).

## Decision

Use [Viper](https://github.com/spf13/viper) for layered configuration with
the following precedence (highest to lowest):

1. CLI flags (via Cobra)
2. Environment variables (`GOKAPI_*` prefix)
3. Project config (`./gokapi.yaml`)
4. User config (`~/.config/gokapi/gokapi.yaml`)
5. System config (`/etc/gokapi/gokapi.yaml`)
6. Code defaults

YAML is the configuration file format. Per-format and per-tool configs are
nested under `formats:` and `tools:` keys:

```yaml
formats:
  html:
    preserveWhitespace: false
tools:
  ai-translation:
    provider: anthropic
    model: claude-sonnet-4-20250514
```

The CLI uses [Cobra](https://github.com/spf13/cobra) for command structure
with hierarchical subcommands (`kapi plugins install`, `kapi flow list`).

## Alternatives Considered

- **TOML**: good for simple configs but less common in the Go ecosystem.
- **JSON**: no comments; verbose.
- **XML** (Okapi style): verbose; poor developer experience.
- **Custom config system**: unnecessary when Viper handles merging, env vars,
  and file watching.

## Consequences

- Environment variables work naturally in CI/CD and Docker
- Project-level configs enable team-shared settings
- CLI flags override everything for one-off runs
- YAML is human-friendly with comment support
- Viper's automatic env binding means `GOKAPI_TOOLS_AI_TRANSLATION_MODEL`
  overrides the nested YAML path
