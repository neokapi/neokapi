package ocr

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func TestLayoutRole(t *testing.T) {
	cases := map[string]struct {
		cls  int
		want string
	}{
		"table":           {21, model.RoleTable},
		"doc_title":       {6, model.RoleTitle},
		"paragraph_title": {17, model.RoleHeading},
		"text":            {22, model.RoleParagraph},
		"image":           {14, model.RolePicture},
		"figure_title":    {7, model.RoleCaption},
		"display_formula": {5, model.RoleFormula},
		"out-of-range-hi": {99, model.RoleParagraph},
		"out-of-range-lo": {-1, model.RoleParagraph},
	}
	for name, c := range cases {
		if got := layoutRole(c.cls); got != c.want {
			t.Errorf("%s: layoutRole(%d) = %q, want %q", name, c.cls, got, c.want)
		}
	}
	if len(layoutLabels) != 25 {
		t.Errorf("layoutLabels = %d, want 25 (PP-DocLayoutV3 classes)", len(layoutLabels))
	}
}
