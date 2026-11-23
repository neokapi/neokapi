// Command undertest prints whether dataroot counts this process as a test
// binary. TestARenamedBinaryIsNotATest builds it under a name ending in
// ".test".
package main

import (
	"fmt"

	"github.com/neokapi/neokapi/core/dataroot"
)

func main() {
	fmt.Println(dataroot.UnderTest())
}
