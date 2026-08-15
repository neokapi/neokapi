# host/venue/schema

The recipe-extension *vocabulary* for the bowrain venue: the typed specs
(`ServerSpec`, `HooksSpec`, `AutomationSpec`, `AssetsSpec`, `BrandVoiceSpec`),
their enum and URL validation, and the YAML decoders that register against the
framework's `core/project` extension mechanism from `init()`.

It is the recipe *format* definition rather than any venue's implementation of
it: a clean leaf over the framework, the standard library and `gopkg.in/yaml.v3`,
with no sync, auth, or server logic. That is what lets a reader of recipes
validate one — Kapi Desktop blank-imports this package to check a venue recipe on
open — without linking the venue's behaviour at all.
