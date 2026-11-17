package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHumanCodeName(t *testing.T) {
	for in, want := range map[string]string{
		"{{name}}":     "name",
		"{{.Name}}":    "Name",
		"{count}":      "count",
		"${value}":     "value",
		"%(user)s":     "user",
		"[B]":          "B",
		"=m0":          "",
		"<=m0>":        "",
		"{=m0}":        "",
		"%1$s":         "",
		"<b>":          "",
		`<a href="x">`: "",
		"":             "",
	} {
		assert.Equal(t, want, HumanCodeName(in), in)
	}
	assert.Equal(t, "label", HumanCodeName("=m3", "label"))
}

func TestCodeKindOf(t *testing.T) {
	assert.Equal(t, CodeKindBold, CodeKindOf("jsx:element", "strong"))
	assert.Equal(t, CodeKindContent, CodeKindOf("jsx:node", ""))
	assert.Equal(t, CodeKindVariable, CodeKindOf("jsx:var", ""))
	assert.Equal(t, CodeKindBreak, CodeKindOf("struct:break", ""))
	assert.Equal(t, CodeKindElement, CodeKindOf("x-link", ""))
	assert.Equal(t, CodeKindLink, CodeKindOf("md:link", ""))
}

func TestPlaceholderWordReadsATagAsItsKind(t *testing.T) {
	assert.Equal(t, "line break", PlaceholderWord(&PlaceholderRun{Type: "html:br", SubType: "br", Data: "<br/>", Equiv: "br"}))
}
