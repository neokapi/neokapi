package tools

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/set"
	"github.com/neokapi/neokapi/core/tool"
)

// RemoveTargetConfig holds configuration for the remove-target tool.
type RemoveTargetConfig struct {
	TargetLocale       model.LocaleID `schema:"title=Target Locale,description=Target locale to remove (if empty all targets are removed)"`                                                   // Target locale to remove (if empty, removes ALL targets)
	TextUnitIDs        string         `schema:"title=Text Unit IDs for Removal,description=Comma-delimited list of text unit IDs whose targets should be removed; empty removes all targets"` // Text unit IDs for selective removal
	FilterByIDs        bool           `schema:"title=Filter by IDs,description=When true filter text units by ID; when false filter by target locale,default=true"`                           // Filter mode: by ID or by locale
	RemoveBlockIfEmpty bool           `schema:"title=Remove Empty Text Units,description=Remove the text unit entirely if it has no remaining targets after removal"`                         // Remove block when no targets remain
}

// ToolName returns the tool name this config applies to.
func (c *RemoveTargetConfig) ToolName() string { return "remove-target" }

// Reset restores default values.
func (c *RemoveTargetConfig) Reset() {
	c.TargetLocale = ""
	c.TextUnitIDs = ""
	c.FilterByIDs = true
	c.RemoveBlockIfEmpty = false
}

// Validate checks configuration validity.
func (c *RemoveTargetConfig) Validate() error {
	return nil
}

// NewRemoveTargetTool creates a new remove-target tool.
// It removes target segments from blocks for a specified locale,
// or all targets if no locale is specified.
func NewRemoveTargetTool(cfg *RemoveTargetConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "remove-target",
		ToolDescription: "Removes a locale's target (or all targets) from blocks",
		Cfg:             cfg,
	}
	// Translate: remove-target modifies/clears targets; source is read-only.
	t.Translate = func(v tool.TargetView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*RemoveTargetConfig)

		// When filtering by IDs, only remove targets for listed text unit IDs.
		if conf.FilterByIDs && conf.TextUnitIDs != "" {
			idSet := set.New[string]()
			for id := range strings.SplitSeq(conf.TextUnitIDs, ",") {
				idSet.Add(strings.TrimSpace(id))
			}
			if !idSet.Contains(v.ID()) {
				return nil
			}
		}

		if conf.TargetLocale.IsEmpty() {
			// Remove all targets.
			v.ClearTargets()
		} else {
			v.RemoveTarget(conf.TargetLocale)
		}

		// Remove block entirely if no targets remain and configured to do so.
		if conf.RemoveBlockIfEmpty && len(v.TargetLocales()) == 0 {
			v.Drop()
		}

		return nil
	}
	return t
}
