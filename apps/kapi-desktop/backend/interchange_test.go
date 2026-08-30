package backend

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Moving store contents in and out as interchange files is the CLI's job
// (`kapi memory`, `kapi terms`). The desktop shows the stores and never binds a
// method that opens a file dialog to load or unload one, so the app build
// carries no interchange path a person can reach.
func TestApp_BindsNoStoreInterchangeMethod(t *testing.T) {
	interchange := regexp.MustCompile(`^(Import|Export)(TMX|Terms)`)

	typ := reflect.TypeOf(NewApp())
	var found []string
	for i := range typ.NumMethod() {
		if name := typ.Method(i).Name; interchange.MatchString(name) {
			found = append(found, name)
		}
	}
	assert.Empty(t, found, "the desktop must bind no store interchange dialog")
}
