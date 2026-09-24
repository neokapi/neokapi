# host/venue/schema

Recipe extension types for a connected venue: `ServerSpec`, `HooksSpec`,
`AutomationSpec`, `AssetsSpec` and `VoiceSpec`. The package provides enum and URL
validation and registers YAML decoders through `core/project` during `init()`.

Its dependencies are limited to the framework, standard library and
`gopkg.in/yaml.v3`. Kapi Desktop blank-imports it to validate recipes without
linking synchronization, authentication or server implementations.
