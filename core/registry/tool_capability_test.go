package registry

import (
	"testing"

	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterRejectsMultiHandlerTool pins the registration-time guard for the
// exactly-one-capability-handler contract (AD-006): a factory whose tool sets
// two typed handlers panics at Register (init-time programmer error) instead
// of silently losing one handler to precedence dispatch.
func TestRegisterRejectsMultiHandlerTool(t *testing.T) {
	multi := func() tool.Tool {
		return &tool.BaseTool{
			ToolName:  "double-agent",
			Annotate:  func(tool.BlockView) error { return nil },
			Transform: func(tool.BlockView) (tool.EditPlan, error) { return tool.EditPlan{}, nil },
		}
	}
	single := func() tool.Tool {
		return &tool.BaseTool{
			ToolName: "annotator",
			Annotate: func(tool.BlockView) error { return nil },
		}
	}
	none := func() tool.Tool { return &tool.BaseTool{ToolName: "pass-through"} }

	t.Run("Register panics on two handlers", func(t *testing.T) {
		r := NewToolRegistry()
		assert.PanicsWithValue(t,
			`registry: register tool "multi": tool "double-agent" sets 2 capability handlers (Annotate, Transform): a BaseTool sets exactly one of Annotate, Produce, or Transform, and the handler is the tool's capability declaration (AD-006)`,
			func() { r.Register("multi", multi) })
	})

	t.Run("RegisterWithSchema panics on two handlers", func(t *testing.T) {
		r := NewToolRegistry()
		assert.Panics(t, func() { r.RegisterWithSchema("multi", multi, nil) })
	})

	t.Run("one or zero handlers register fine", func(t *testing.T) {
		r := NewToolRegistry()
		assert.NotPanics(t, func() {
			r.Register("single", single)
			r.Register("none", none)
		})
	})

	t.Run("NewToolWithConfig rejects a config-built multi-handler tool", func(t *testing.T) {
		r := NewToolRegistry()
		r.Register("configurable", single)
		r.SetConfigFactory("configurable", func(config map[string]any, targetLang string) (tool.Tool, error) {
			return multi(), nil
		})
		_, err := r.NewToolWithConfig("configurable", nil, "")
		require.ErrorContains(t, err, "exactly one")
	})
}
