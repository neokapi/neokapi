// Command list prints the container images the bowrain suites start, one per
// line, for the CI step that fetches them before the tests run.
package main

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/testutil/testimage"
)

func main() {
	for _, img := range testimage.All() {
		fmt.Println(img)
	}
}
