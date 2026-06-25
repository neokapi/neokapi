# bowrain/plugin/schema

**License: Apache-2.0** (see `LICENSE` in this directory).

This submodule is licensed Apache-2.0, **distinct from the AGPL-3.0 `bowrain/`
tree it nests under**. It is the recipe-extension *vocabulary* for bowrain: the
typed specs (`ServerSpec`, `HooksSpec`, `AutomationSpec`, `AssetsSpec`,
`BrandVoiceSpec`), their enum/URL validation, and the YAML decoders that
register against the framework's `core/project` extension mechanism via
`init()`.

It is a clean leaf — it imports only the Apache-2.0 framework
(`github.com/neokapi/neokapi`), the standard library, and `gopkg.in/yaml.v3`. It
does **not** import any AGPL `bowrain/*` package and contains no platform,
sync, auth, or server logic.

Because it is the recipe *format* definition rather than bowrain's
implementation, Apache consumers may depend on it. In particular **Kapi Desktop**
(Apache-2.0) blank-imports this package to validate bowrain recipes on open,
without pulling in any AGPL code.
