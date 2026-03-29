package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/dop251/goja"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// ScriptConfig holds configuration for the script tool.
type ScriptConfig struct {
	Code       string // Inline JavaScript code
	ScriptFile string // Path to a .js file (alternative to inline Code)
}

// ToolName returns the tool name this config applies to.
func (c *ScriptConfig) ToolName() string { return "script" }

// Reset restores default values.
func (c *ScriptConfig) Reset() {
	c.Code = ""
	c.ScriptFile = ""
}

// Validate checks configuration validity.
func (c *ScriptConfig) Validate() error {
	if c.Code == "" && c.ScriptFile == "" {
		return fmt.Errorf("script: either Code or ScriptFile is required")
	}
	if c.Code != "" && c.ScriptFile != "" {
		return fmt.Errorf("script: Code and ScriptFile are mutually exclusive")
	}
	return nil
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
	return t
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
		emittedPart := jsToPartUpdate(s.vm, obj, part)
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

	_, err := s.vm.RunProgram(s.program)
	if err != nil {
		return nil, err
	}

	if skipped {
		return nil, nil
	}
	if !emitCalled {
		return []*model.Part{part}, nil
	}
	return emitted, nil
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

	// Source segments as array of {content: {text: "..."}}
	source := make([]any, 0, len(block.Source))
	for _, seg := range block.Source {
		segObj := vm.NewObject()
		contentObj := vm.NewObject()
		_ = contentObj.Set("text", seg.Content.Text())
		_ = segObj.Set("content", contentObj)
		source = append(source, segObj)
	}
	_ = obj.Set("source", source)

	// Targets as map of locale -> [{content: {text: "..."}}]
	targets := vm.NewObject()
	for locale, segs := range block.Targets {
		localeSegs := make([]any, 0, len(segs))
		for _, seg := range segs {
			segObj := vm.NewObject()
			contentObj := vm.NewObject()
			_ = contentObj.Set("text", seg.Content.Text())
			_ = segObj.Set("content", contentObj)
			localeSegs = append(localeSegs, segObj)
		}
		_ = targets.Set(string(locale), localeSegs)
	}
	_ = obj.Set("targets", targets)

	return obj
}

// jsToPartUpdate reads back modified data from the JS object and applies
// changes to a clone of the original Part. Only block text changes are applied.
func jsToPartUpdate(vm *goja.Runtime, obj *goja.Object, original *model.Part) *model.Part {
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
						if text != block.SourceText() {
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
