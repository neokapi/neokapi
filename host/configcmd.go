package host

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/output"
)

// One config verb.
//
// `kapi config` is the only configuration command in the CLI. Plugins do not
// ship their own: a plugin claims a dotted namespace in its manifest
// (capabilities.config_namespaces) and kapi routes `kapi config set
// bowrain.server.url …` to that plugin's own global config file, which the
// plugin then reads exactly as it always did. Recipe fields keep the
// positional form (`kapi config server.url …`), which edits the project's
// kapi.yaml — a different scope, on purpose: app config is per-machine, the
// recipe is committed.

// ConfigEntry is one configuration key in a listing or a get/set/unset result.
type ConfigEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Scope is "app" for kapi's own global config, or the plugin namespace
	// prefix (e.g. "bowrain") for a plugin-owned key.
	Scope string `json:"scope"`
	// File is the config file the key is stored in.
	File string `json:"file,omitempty"`
	// Description documents a key a plugin declared in its manifest.
	Description string `json:"description,omitempty"`
}

// ConfigResult is the structured result of a single-key config operation.
type ConfigResult struct {
	Action string      `json:"action"` // get | set | unset | path
	Entry  ConfigEntry `json:"entry"`
	// Changed reports whether a set/unset actually altered stored state.
	Changed bool `json:"changed,omitempty"`
}

// FormatText renders a single-key result the way a shell user expects.
func (r ConfigResult) FormatText(w io.Writer) error {
	switch r.Action {
	case "get":
		fmt.Fprintln(w, r.Entry.Value)
	case "path":
		fmt.Fprintln(w, r.Entry.File)
	case "set":
		fmt.Fprintf(w, "set %s = %s (%s)\n", r.Entry.Key, r.Entry.Value, DisplayName(r.Entry.File))
	case "unset":
		if r.Changed {
			fmt.Fprintf(w, "unset %s (%s)\n", r.Entry.Key, DisplayName(r.Entry.File))
		} else {
			fmt.Fprintf(w, "%s was not set\n", r.Entry.Key)
		}
	}
	return nil
}

// ConfigListOutput is the structured result of `kapi config list`.
type ConfigListOutput struct {
	Entries []ConfigEntry `json:"entries"`
}

// FormatText renders the listing as `key = value` lines, grouped by scope so
// plugin-owned keys are visibly separate from kapi's own.
func (o ConfigListOutput) FormatText(w io.Writer) error {
	if len(o.Entries) == 0 {
		fmt.Fprintln(w, "No configuration set.")
		return nil
	}
	t := output.NewTable(w).Accent(0).Headers("KEY", "VALUE", "SCOPE")
	for _, e := range o.Entries {
		t.Row(e.Key, e.Value, e.Scope)
	}
	t.Render()
	return nil
}

// configScope is a resolved destination for one key: which file backs it, and
// what the key is called inside that file.
type configScope struct {
	scope       string // "app" or a plugin namespace prefix
	app         string // config app name ("" = kapi)
	storedKey   string // key as written in the backing file
	description string
}

// resolveConfigScope routes a user-typed key to its backing store. A key whose
// first dotted segment matches an installed plugin's declared namespace belongs
// to that plugin; everything else is kapi's own app config.
func (a *App) resolveConfigScope(key string) configScope {
	prefix, rest, ok := strings.Cut(key, ".")
	if ok && a.PluginHost != nil {
		if route := a.PluginHost.ConfigNamespaceRoute(prefix); route != nil {
			desc := ""
			for _, k := range route.Namespace.Keys {
				if strings.EqualFold(k.Key, rest) {
					desc = k.Description
					break
				}
			}
			return configScope{scope: prefix, app: route.App(), storedKey: rest, description: desc}
		}
	}
	return configScope{scope: "app", storedKey: key}
}

// appArgs adapts a scope to the variadic appName argument of the config
// helpers ("" means kapi's own file).
func (s configScope) appArgs() []string {
	if s.app == "" {
		return nil
	}
	return []string{s.app}
}

// ConfigGet reads one key from whichever store owns it.
func (a *App) ConfigGet(key string) (ConfigResult, error) {
	s := a.resolveConfigScope(key)
	entry := ConfigEntry{
		Key:         key,
		Scope:       s.scope,
		File:        config.GlobalConfigFilePath(s.appArgs()...),
		Description: s.description,
	}
	if s.scope == "app" && a.Config != nil {
		// kapi's own keys read through the loaded AppConfig so env-var
		// overrides (KAPI_AI_PROVIDER, …) and built-in defaults are visible —
		// `get` should answer "what will kapi use", not "what is on disk".
		entry.Value = a.Config.GetString(s.storedKey)
		return ConfigResult{Action: "get", Entry: entry}, nil
	}
	values, err := config.GlobalConfigValues(s.appArgs()...)
	if err != nil {
		return ConfigResult{}, err
	}
	entry.Value = values[strings.ToLower(s.storedKey)]
	return ConfigResult{Action: "get", Entry: entry}, nil
}

// ConfigSet writes one key to whichever store owns it.
func (a *App) ConfigSet(key, value string) (ConfigResult, error) {
	s := a.resolveConfigScope(key)
	if err := config.SetGlobalConfig(s.storedKey, value, s.appArgs()...); err != nil {
		return ConfigResult{}, fmt.Errorf("set %s: %w", key, err)
	}
	if s.scope == "app" && a.Config != nil {
		// Mirror into the loaded config so a later step in the same process
		// (an inline wizard, a chained command) sees the new value.
		a.Config.Set(s.storedKey, value)
	}
	return ConfigResult{
		Action:  "set",
		Changed: true,
		Entry: ConfigEntry{
			Key: key, Value: value, Scope: s.scope,
			File:        config.GlobalConfigFilePath(s.appArgs()...),
			Description: s.description,
		},
	}, nil
}

// ConfigUnset removes one key, restoring kapi's built-in default.
func (a *App) ConfigUnset(key string) (ConfigResult, error) {
	s := a.resolveConfigScope(key)
	changed, err := config.UnsetGlobalConfig(s.storedKey, s.appArgs()...)
	if err != nil {
		return ConfigResult{}, fmt.Errorf("unset %s: %w", key, err)
	}
	return ConfigResult{
		Action:  "unset",
		Changed: changed,
		Entry: ConfigEntry{
			Key: key, Scope: s.scope,
			File:        config.GlobalConfigFilePath(s.appArgs()...),
			Description: s.description,
		},
	}, nil
}

// ConfigPath reports the file backing a namespace — `kapi config path` for
// kapi's own config, `kapi config path bowrain` for a plugin's.
func (a *App) ConfigPath(namespace string) ConfigResult {
	s := configScope{scope: "app"}
	if namespace != "" {
		s = a.resolveConfigScope(namespace + ".")
	}
	return ConfigResult{
		Action: "path",
		Entry:  ConfigEntry{Scope: s.scope, File: config.GlobalConfigFilePath(s.appArgs()...)},
	}
}

// ConfigList enumerates every persisted key: kapi's own, plus each installed
// plugin's namespace re-prefixed so the listed key is exactly what a user would
// type back into `kapi config set`.
func (a *App) ConfigList() (ConfigListOutput, error) {
	var out ConfigListOutput

	appFile := config.GlobalConfigFilePath()
	appValues, err := config.GlobalConfigValues()
	if err != nil {
		return out, err
	}
	for _, k := range sortedKeys(appValues) {
		out.Entries = append(out.Entries, ConfigEntry{
			Key: k, Value: appValues[k], Scope: "app", File: appFile,
		})
	}

	if a.PluginHost == nil {
		return out, nil
	}
	for _, route := range a.PluginHost.ConfigNamespaceRoutes() {
		file := config.GlobalConfigFilePath(route.App())
		values, verr := config.GlobalConfigValues(route.App())
		if verr != nil {
			return out, verr
		}
		described := map[string]string{}
		for _, k := range route.Namespace.Keys {
			described[strings.ToLower(k.Key)] = k.Description
		}
		for _, k := range sortedKeys(values) {
			out.Entries = append(out.Entries, ConfigEntry{
				Key:         route.Namespace.Prefix + "." + k,
				Value:       values[k],
				Scope:       route.Namespace.Prefix,
				File:        file,
				Description: described[k],
			})
		}
	}
	return out, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ConfigNamespaceHint lists the plugin-owned config namespaces for help text
// and error messages, so an unknown key can point at what does exist.
func (a *App) ConfigNamespaceHint() string {
	if a.PluginHost == nil {
		return ""
	}
	routes := a.PluginHost.ConfigNamespaceRoutes()
	if len(routes) == 0 {
		return ""
	}
	names := make([]string, 0, len(routes))
	for _, r := range routes {
		names = append(names, r.Namespace.Prefix+".*")
	}
	return strings.Join(names, ", ")
}
