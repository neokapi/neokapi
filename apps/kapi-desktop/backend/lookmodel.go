package backend

// The Map asks whether a model is connected each time it is shown. The
// credential lookup behind the answer reads the saved key from the OS keychain
// once per tool the deeper look builds. lookModelCache keeps the answer for
// the session, so showing the Map again reads no key.
//
// The App drops the kept answers when a provider key or the default model
// changes, and when a deeper look finds no model to ask. An answer older than
// lookModelTTL is read again, so a key removed by `kapi credentials` in a
// terminal stops reading as connected within that time.

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	appconfig "github.com/neokapi/neokapi/host/config"
)

// lookModelTTL is how long a kept answer stands before the next question
// reads the keychain again.
const lookModelTTL = 10 * time.Minute

// lookModelCache holds the answer DeeperLookModel gives, per resolution input.
// The zero value is ready to use.
type lookModelCache struct {
	mu sync.Mutex
	// gen counts invalidations. A lookup that began before one keeps nothing,
	// so a key written while the keychain was being read is not shadowed by
	// the answer from before the write.
	gen     uint64
	entries map[string]lookModelEntry

	// resolve and now replace host.App.ResolveDeeperLookModel and time.Now
	// in tests; nil means the real ones.
	resolve func(*project.KapiProject) (provider, model string, err error)
	now     func() time.Time
}

type lookModelEntry struct {
	model DeeperLookModelDTO
	at    time.Time
}

// lookModelKey names what the answer depends on besides the saved keys and
// the environment: the recipe's tool presets, which may pin a provider, a
// model or a credential, and the default model in the app config. Two tabs on
// recipes that agree share one answer. ok is false when the presets do not
// encode, and the caller then asks without keeping the answer.
func lookModelKey(cfg *appconfig.AppConfig, proj *project.KapiProject) (key string, ok bool) {
	in := struct {
		Tools   map[string]map[string]any `json:"tools,omitempty"`
		Default appconfig.AIDefault       `json:"default"`
	}{Default: appconfig.ResolveAIDefault(cfg)}
	if proj != nil {
		in.Tools = proj.Defaults.Tools
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// lookModel answers DeeperLookModel for proj, from the kept answer when there
// is a current one.
func (a *App) lookModel(proj *project.KapiProject) DeeperLookModelDTO {
	c := &a.lookModels
	key, keep := lookModelKey(a.aiConfig, proj)

	c.mu.Lock()
	now := c.clock()
	if e, found := c.entries[key]; keep && found && now.Sub(e.at) < lookModelTTL {
		c.mu.Unlock()
		return e.model
	}
	gen := c.gen
	resolve := c.resolve
	c.mu.Unlock()

	// The keychain read happens outside the lock, so a key written while a
	// keychain prompt is open does not wait on the prompt.
	if resolve == nil {
		resolve = (&host.App{Credentials: a.credentials, Config: a.aiConfig}).ResolveDeeperLookModel
	}
	var model DeeperLookModelDTO
	if provider, name, err := resolve(proj); err == nil {
		model = DeeperLookModelDTO{Connected: true, Provider: provider, Model: name}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if keep && c.gen == gen {
		if c.entries == nil {
			c.entries = make(map[string]lookModelEntry)
		}
		c.entries[key] = lookModelEntry{model: model, at: now}
	}
	return model
}

// forgetLookModels drops every kept answer, so the next question reads the
// saved keys again.
func (a *App) forgetLookModels() {
	c := &a.lookModels
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	clear(c.entries)
}

func (c *lookModelCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
