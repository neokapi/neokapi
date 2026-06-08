package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/dop251/goja"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ScriptConfig holds configuration for the script tool.
// Source selects between inline code and file — a standard mode-selector pattern.
type ScriptConfig struct {
	Source     string `json:"source,omitempty"     schema:"title=Script Source,description=Script source mode,enum=inline|file,default=inline,widget=segmented"`
	Code       string `json:"code,omitempty"       schema:"title=Inline Code,description=Inline ES5 JavaScript code,widget=code-editor,showIf=source:inline"`
	ScriptFile string `json:"scriptFile,omitempty" schema:"title=Script File,description=Path to a .js file,widget=file-picker,showIf=source:file"`
	// AllowSourceMutation opts the script into writing the block's SOURCE.
	// Off by default: source is read-only to the script (its source edits are
	// ignored), matching the content-model immutability contract; only target
	// edits take effect. Enable for source-transform scripts that run early,
	// before any segmentation/term/entity overlay is attached.
	AllowSourceMutation bool `json:"allowSourceMutation,omitempty" schema:"title=Allow Source Mutation,description=Permit the script to modify the source text (off by default; source is read-only)"`
}

// ToolName returns the tool name this config applies to.
func (c *ScriptConfig) ToolName() string { return "script" }

// Reset restores default values.
func (c *ScriptConfig) Reset() {
	c.Source = "inline"
	c.Code = ""
	c.ScriptFile = ""
	c.AllowSourceMutation = false
}

// Validate checks configuration validity.
func (c *ScriptConfig) Validate() error {
	switch c.Source {
	case "file", "File":
		if c.ScriptFile == "" {
			return errors.New("script: ScriptFile is required when source is 'file'")
		}
	default: // "inline" or empty
		if c.Code == "" {
			return errors.New("script: Code is required when source is 'inline'")
		}
	}
	return nil
}

// ScriptSchema returns the auto-generated schema for the script tool.
func ScriptSchema() *schema.ComponentSchema {
	return schema.FromStruct(&ScriptConfig{}, schema.ToolMeta{
		ID:          "script",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Script",
		Description: "Run a JavaScript processing script on each part",
	})
}

// NewScriptFromConfig creates a script tool from a config map.
func NewScriptFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg ScriptConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("script config: %w", err)
	}
	if cfg.Code == "" && cfg.ScriptFile == "" {
		return nil, errors.New("either --code or --script-file is required")
	}
	return NewScriptTool(&cfg), nil
}

// ScriptTool runs user-provided JavaScript (ES5) via goja.
// Each instance owns its own goja.Runtime (safe -- one goroutine per tool instance).
type ScriptTool struct {
	tool.BaseTool
	program *goja.Program
	vm      *goja.Runtime
}

// NewScriptTool creates a new script tool with the given configuration.
func NewScriptTool(cfg *ScriptConfig) *ScriptTool {
	t := &ScriptTool{}
	t.ToolName = "script"
	t.ToolDescription = "Run a JavaScript processing script on each part"
	t.Cfg = cfg
	// script is the general-purpose tool: it overrides Process (for emit/skip
	// and the function-form return), so it is exempt from the dispatch's typed
	// block handlers. It self-enforces immutability instead — jsToPartUpdate
	// applies source edits only when allowsSourceMutation() is set; targets are
	// always writable.
	return t
}

// allowsSourceMutation reports whether the script is permitted to write the
// block's source. Off by default — source is read-only to the script.
func (s *ScriptTool) allowsSourceMutation() bool {
	c, _ := s.Cfg.(*ScriptConfig)
	return c != nil && c.AllowSourceMutation
}

// Process runs the compiled JavaScript program for each Part from the input channel.
func (s *ScriptTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	if err := s.init(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			emitted, err := s.runScript(part)
			if err != nil {
				return fmt.Errorf("script: %w", err)
			}
			for _, p := range emitted {
				select {
				case out <- p:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// init lazily compiles the script and creates the goja runtime.
func (s *ScriptTool) init() error {
	if s.vm != nil {
		return nil
	}

	cfg := s.Cfg.(*ScriptConfig)
	code := cfg.Code
	if code == "" && cfg.ScriptFile != "" {
		data, err := os.ReadFile(cfg.ScriptFile)
		if err != nil {
			return fmt.Errorf("script: reading script file: %w", err)
		}
		code = string(data)
	}

	// An empty script is valid -- it passes all parts through.
	if code == "" {
		s.vm = goja.New()
		return nil
	}

	prog, err := goja.Compile("script", code, false)
	if err != nil {
		return fmt.Errorf("script: compile error: %w", err)
	}
	s.program = prog
	s.vm = goja.New()
	return nil
}

// runScript executes the compiled program for a single Part and returns emitted parts.
func (s *ScriptTool) runScript(part *model.Part) ([]*model.Part, error) {
	// If no program was compiled (empty script), pass through.
	if s.program == nil {
		return []*model.Part{part}, nil
	}

	jsObj := partToJS(s.vm, part)
	_ = s.vm.Set("part", jsObj)

	var emitted []*model.Part
	skipped := false
	emitCalled := false

	_ = s.vm.Set("emit", func(call goja.FunctionCall) goja.Value {
		emitCalled = true
		arg := call.Argument(0)
		if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
			return goja.Undefined()
		}
		obj := arg.ToObject(s.vm)
		emittedPart := jsToPartUpdate(s.vm, obj, part, s.allowsSourceMutation())
		emitted = append(emitted, emittedPart)
		return goja.Undefined()
	})

	_ = s.vm.Set("skip", func(call goja.FunctionCall) goja.Value {
		skipped = true
		return goja.Undefined()
	})

	_ = s.vm.Set("log", func(call goja.FunctionCall) goja.Value {
		msg := call.Argument(0).String()
		fmt.Fprintln(os.Stderr, msg)
		return goja.Undefined()
	})

	// Run the program. In the implicit-globals form this executes the logic
	// against `part`; in the function form it (re)defines process().
	if _, err := s.vm.RunProgram(s.program); err != nil {
		return nil, err
	}

	// Function form: if the script defined `function process(part) { … }`, call
	// it for this Part. emit()/skip() inside it work exactly as in the globals
	// form; as a convenience the return value is also honored — return a part
	// (or an array of parts) to emit, return null to drop it, return nothing to
	// pass it through unchanged.
	if pv := s.vm.Get("process"); pv != nil {
		if processFn, ok := goja.AssertFunction(pv); ok {
			ret, err := processFn(goja.Undefined(), jsObj)
			if err != nil {
				return nil, err
			}
			if !skipped && !emitCalled && ret != nil {
				switch {
				case goja.IsNull(ret):
					skipped = true
				case goja.IsUndefined(ret):
					// Pass-through: handled by the !emitCalled branch below.
				default:
					emitted = append(emitted, s.returnedParts(ret, part)...)
					emitCalled = true
				}
			}
		}
	}

	if skipped {
		return nil, nil
	}
	if !emitCalled {
		return []*model.Part{part}, nil
	}
	return emitted, nil
}

// returnedParts converts a process() return value into emitted parts, applying
// any edits back onto the original Part. The value may be a single part object
// or an array of them.
func (s *ScriptTool) returnedParts(v goja.Value, original *model.Part) []*model.Part {
	obj := v.ToObject(s.vm)
	if obj.ClassName() != "Array" {
		return []*model.Part{jsToPartUpdate(s.vm, obj, original, s.allowsSourceMutation())}
	}
	var out []*model.Part
	length := 0
	if n := obj.Get("length"); n != nil {
		length = int(n.ToInteger())
	}
	for i := range length {
		el := obj.Get(strconv.Itoa(i))
		if el == nil || goja.IsUndefined(el) || goja.IsNull(el) {
			continue
		}
		out = append(out, jsToPartUpdate(s.vm, el.ToObject(s.vm), original, s.allowsSourceMutation()))
	}
	return out
}

// partTypeString returns a JS-friendly string for a PartType.
func partTypeString(pt model.PartType) string {
	switch pt {
	case model.PartBlock:
		return "block"
	case model.PartData:
		return "data"
	case model.PartMedia:
		return "media"
	case model.PartLayerStart:
		return "layer-start"
	case model.PartLayerEnd:
		return "layer-end"
	case model.PartGroupStart:
		return "group-start"
	case model.PartGroupEnd:
		return "group-end"
	default:
		return "unknown"
	}
}

// partToJS converts a Part into a JS-friendly goja object.
func partToJS(vm *goja.Runtime, part *model.Part) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("type", partTypeString(part.Type))

	if part.Type == model.PartBlock {
		block, ok := part.Resource.(*model.Block)
		if ok {
			_ = obj.Set("block", blockToJS(vm, block))
		}
	}
	return obj
}

// blockToJS converts a Block into a JS-friendly goja object.
func blockToJS(vm *goja.Runtime, block *model.Block) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("id", block.ID)
	_ = obj.Set("translatable", block.Translatable)

	// Source content as a native JS one-element array of {content: {text}}.
	// vm.NewArray (rather than a Go []any) makes in-place edits such as
	// part.block.source[0].content.text = "..." round-trip through Export; a
	// block's content is now a single run sequence rather than N segments.
	srcSeg := vm.NewObject()
	srcContent := vm.NewObject()
	_ = srcContent.Set("text", block.SourceText())
	_ = srcSeg.Set("content", srcContent)
	_ = obj.Set("source", vm.NewArray(srcSeg))

	// Targets as a map of locale -> native JS one-element array.
	targets := vm.NewObject()
	for _, locale := range block.TargetLocales() {
		tSeg := vm.NewObject()
		tContent := vm.NewObject()
		_ = tContent.Set("text", block.TargetText(locale))
		_ = tSeg.Set("content", tContent)
		_ = targets.Set(string(locale), vm.NewArray(tSeg))
	}
	_ = obj.Set("targets", targets)

	return obj
}

// jsToPartUpdate reads back modified data from the JS object and applies
// changes to a clone of the original Part. Only block text changes are applied.
func jsToPartUpdate(vm *goja.Runtime, obj *goja.Object, original *model.Part, allowSourceMutation bool) *model.Part {
	if original.Type != model.PartBlock {
		return original
	}

	block, ok := original.Resource.(*model.Block)
	if !ok {
		return original
	}

	blockVal := obj.Get("block")
	if blockVal == nil || goja.IsUndefined(blockVal) || goja.IsNull(blockVal) {
		return original
	}
	jsBlock := blockVal.ToObject(vm)

	// Check if source text was modified.
	sourceVal := jsBlock.Get("source")
	if sourceVal != nil && !goja.IsUndefined(sourceVal) && !goja.IsNull(sourceVal) {
		sourceArr := sourceVal.Export()
		if segs, ok := sourceArr.([]any); ok && len(segs) > 0 {
			if segMap, ok := segs[0].(map[string]any); ok {
				if contentMap, ok := segMap["content"].(map[string]any); ok {
					if text, ok := contentMap["text"].(string); ok {
						// Source is read-only unless the script opts in; otherwise
						// its source edits are ignored (immutability contract).
						if allowSourceMutation && text != block.SourceText() {
							block.SetSourceText(text)
						}
					}
				}
			}
		}
	}

	// Check if targets were modified.
	targetsVal := jsBlock.Get("targets")
	if targetsVal != nil && !goja.IsUndefined(targetsVal) && !goja.IsNull(targetsVal) {
		targetsMap := targetsVal.Export()
		if tm, ok := targetsMap.(map[string]any); ok {
			for locale, localeSegs := range tm {
				if segs, ok := localeSegs.([]any); ok && len(segs) > 0 {
					if segMap, ok := segs[0].(map[string]any); ok {
						if contentMap, ok := segMap["content"].(map[string]any); ok {
							if text, ok := contentMap["text"].(string); ok {
								block.SetTargetText(model.LocaleID(locale), text)
							}
						}
					}
				}
			}
		}
	}

	return original
}
