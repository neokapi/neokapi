package event

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ContextActorKind is the class of actor behind a context change. It uses
// the vocabulary of contextop.ActorKind.
type ContextActorKind string

// The actor kinds a context change names.
const (
	ContextActorPerson ContextActorKind = "person"
	ContextActorAgent  ContextActorKind = "agent"
	ContextActorTool   ContextActorKind = "tool"
)

// ContextItemKind names what a context change is about.
type ContextItemKind string

// The item kinds a context change names.
const (
	// ContextItemTerm is a concept or term in the terms store.
	ContextItemTerm ContextItemKind = "term"
	// ContextItemTermRule is a term rule in the context ledger.
	ContextItemTermRule ContextItemKind = "term_rule"
	// ContextItemVoiceRule is a rule promoted into, or declined for, a voice
	// profile.
	ContextItemVoiceRule ContextItemKind = "voice_rule"
	// ContextItemVoiceProfile is a whole voice profile.
	ContextItemVoiceProfile ContextItemKind = "voice_profile"
	// ContextItemDecision is a ledger operation with no subject of its own,
	// such as a reset.
	ContextItemDecision ContextItemKind = "decision"
	// ContextItemNote is a note in the context ledger.
	ContextItemNote ContextItemKind = "note"
	// ContextItemMemory is a source and target pair for the content memory.
	ContextItemMemory ContextItemKind = "memory"
	// ContextItemBinding is the binding of a project's context to a project
	// key, made by the first push that names one.
	ContextItemBinding ContextItemKind = "binding"
)

// ContextOperation is what a context change did to its items.
type ContextOperation string

// The operations a context change names.
const (
	ContextOpAdd    ContextOperation = "add"
	ContextOpEdit   ContextOperation = "edit"
	ContextOpRetire ContextOperation = "retire"
	ContextOpKeep   ContextOperation = "keep"
	ContextOpDrop   ContextOperation = "drop"
)

// ContextCounts counts the items of a context change by item kind and
// operation.
type ContextCounts map[ContextItemKind]map[ContextOperation]int

// Add counts n items of kind under op.
func (c ContextCounts) Add(kind ContextItemKind, op ContextOperation, n int) {
	if n <= 0 {
		return
	}
	if c[kind] == nil {
		c[kind] = map[ContextOperation]int{}
	}
	c[kind][op] += n
}

// Total is the number of items counted.
func (c ContextCounts) Total() int {
	total := 0
	for _, ops := range c {
		for _, n := range ops {
			total += n
		}
	}
	return total
}

// only returns the one item kind and the one operation every count shares,
// or empty strings when the counts hold more than one of either.
func (c ContextCounts) only() (ContextItemKind, ContextOperation) {
	if len(c) != 1 {
		return "", ""
	}
	for kind, ops := range c {
		if len(ops) != 1 {
			return kind, ""
		}
		for op := range ops {
			return kind, op
		}
	}
	return "", ""
}

// ContextChange is the typed payload of a context change event: who made the
// change, in which project, what it touched and how many items. Data and
// ParseContextChange convert it to and from an event's Data map, whose keys
// are snake_case.
type ContextChange struct {
	// WorkspaceSlug files the activity row under the workspace the feed
	// lists by.
	WorkspaceSlug string
	// ActorKind is person, agent or tool.
	ActorKind ContextActorKind
	// ActorName is the name the actor is shown by.
	ActorName string
	// Session is the agent session behind the change, when there is one.
	Session string
	// Source names the write path: "push", "decide" or "voice".
	Source string
	// ItemKind and Operation describe the change when every item shares
	// them. Data fills them from Counts when they are empty.
	ItemKind  ContextItemKind
	Operation ContextOperation
	// ItemID names the item when the change concerns one: the operation a
	// decision answered, a voice profile id, a rule's term.
	ItemID string
	// Counts holds the number of items by kind and operation.
	Counts ContextCounts
	// Extra carries further event-specific detail, written beside the typed
	// fields.
	Extra map[string]string
}

// Data renders the change as an event's Data map.
func (c ContextChange) Data() map[string]string {
	data := map[string]string{}
	maps.Copy(data, c.Extra)
	put := func(k, v string) {
		if v != "" {
			data[k] = v
		}
	}
	kind, op := c.ItemKind, c.Operation
	if kind == "" && op == "" {
		kind, op = c.Counts.only()
	}
	put("workspace_slug", c.WorkspaceSlug)
	put("actor_kind", string(c.ActorKind))
	put("actor_name", c.ActorName)
	put("session", c.Session)
	put("source", c.Source)
	put("item_kind", string(kind))
	put("item_id", c.ItemID)
	put("operation", string(op))
	if total := c.Counts.Total(); total > 0 {
		data["count"] = strconv.Itoa(total)
		if raw, err := json.Marshal(c.Counts); err == nil {
			data["counts"] = string(raw)
		}
	}
	return data
}

// ParseContextChange reads a context change back from an event's Data map.
// A counts value that does not parse is read as no counts.
func ParseContextChange(data map[string]string) ContextChange {
	c := ContextChange{
		WorkspaceSlug: data["workspace_slug"],
		ActorKind:     ContextActorKind(data["actor_kind"]),
		ActorName:     data["actor_name"],
		Session:       data["session"],
		Source:        data["source"],
		ItemKind:      ContextItemKind(data["item_kind"]),
		ItemID:        data["item_id"],
		Operation:     ContextOperation(data["operation"]),
		Counts:        ContextCounts{},
	}
	if raw := data["counts"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.Counts); err != nil {
			c.Counts = ContextCounts{}
		}
	}
	return c
}

// Describe renders the counts as a short line, such as "kept 1 term rule ·
// dropped 2 notes", ordered by operation and then by item kind.
func (c ContextCounts) Describe() string {
	type entry struct {
		op   ContextOperation
		kind ContextItemKind
		n    int
	}
	var entries []entry
	for kind, ops := range c {
		for op, n := range ops {
			entries = append(entries, entry{op, kind, n})
		}
	}
	order := []ContextOperation{ContextOpAdd, ContextOpEdit, ContextOpKeep, ContextOpDrop, ContextOpRetire}
	slices.SortFunc(entries, func(a, b entry) int {
		if d := slices.Index(order, a.op) - slices.Index(order, b.op); d != 0 {
			return d
		}
		return strings.Compare(string(a.kind), string(b.kind))
	})
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, contextVerb(e.op)+" "+strconv.Itoa(e.n)+" "+contextNoun(e.kind, e.n))
	}
	return strings.Join(parts, " · ")
}

// contextVerb is the past tense of an operation.
func contextVerb(op ContextOperation) string {
	switch op {
	case ContextOpAdd:
		return "added"
	case ContextOpEdit:
		return "edited"
	case ContextOpKeep:
		return "kept"
	case ContextOpDrop:
		return "dropped"
	case ContextOpRetire:
		return "retired"
	}
	return string(op)
}

// contextNoun names n items of kind.
func contextNoun(kind ContextItemKind, n int) string {
	noun := strings.ReplaceAll(string(kind), "_", " ")
	if kind == ContextItemMemory {
		noun = "memory pair"
	}
	if n != 1 {
		noun += "s"
	}
	return noun
}
