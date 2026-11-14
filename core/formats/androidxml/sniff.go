package androidxml

import (
	"bytes"
	"encoding/xml"
	"io"
)

// Sniff reports whether data, the head of a file, is an Android string
// resources file: its root element is <resources>, and one of the root's
// children is a <string>, <plurals> or <string-array>.
//
// The check reads the element structure, not substrings, so a <resources>
// element below the root (a Maven pom's build/resources) or a <string> outside
// the root does not count. A head that ends before such a child appears is
// not claimed, and the generic xml reader reads the file.
func Sniff(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	// Only the element names matter here, and they are ASCII, so the bytes
	// are read as they are whatever encoding the prolog declares.
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }

	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch depth {
			case 1:
				if t.Name.Local != "resources" || t.Name.Space != "" {
					return false
				}
			case 2:
				switch t.Name.Local {
				case "string", "plurals", "string-array":
					if t.Name.Space == "" {
						return true
					}
				}
			}
		case xml.EndElement:
			depth--
			if depth <= 0 {
				return false
			}
		}
	}
}
