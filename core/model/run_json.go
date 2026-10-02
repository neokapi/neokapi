package model

import (
	"slices"
	"strconv"
	"unicode/utf8"
)

// appendRunJSON appends the JSON Run.MarshalJSON writes for r, built directly
// rather than through reflection: an edition revision hashes these bytes for
// every write, and the reflective encoder costs more than the rest of an edit.
// It reports false, appending nothing, when r or a run in one of its branches
// is not a valid union, which Run.MarshalJSON refuses. TestAppendRunJSON pins
// it to Run.MarshalJSON byte for byte.
func appendRunJSON(dst []byte, r Run) ([]byte, bool) {
	if !runTreeValid(r) {
		return dst, false
	}
	return appendValidRun(dst, r), true
}

func runTreeValid(r Run) bool {
	if !r.Valid() {
		return false
	}
	switch {
	case r.Plural != nil:
		for _, form := range r.Plural.Forms {
			for _, x := range form {
				if !runTreeValid(x) {
					return false
				}
			}
		}
	case r.Select != nil:
		for _, c := range r.Select.Cases {
			for _, x := range c {
				if !runTreeValid(x) {
					return false
				}
			}
		}
	}
	return true
}

func appendValidRun(dst []byte, r Run) []byte {
	switch {
	case r.Text != nil:
		dst = append(dst, `{"text":`...)
		dst = appendJSONString(dst, r.Text.Text)
		if r.Text.NoTranslate {
			dst = append(dst, `,"noTranslate":true`...)
		}
		return append(dst, '}')
	case r.Ph != nil:
		p := r.Ph
		dst = append(dst, `{"ph":`...)
		dst = appendCodeJSON(dst, p.ID, p.Type, p.SubType, p.Data, p.Equiv, p.Disp, p.Attrs, p.Constraints)
		return append(dst, '}')
	case r.PcOpen != nil:
		p := r.PcOpen
		dst = append(dst, `{"pcOpen":`...)
		dst = appendCodeJSON(dst, p.ID, p.Type, p.SubType, p.Data, p.Equiv, p.Disp, p.Attrs, p.Constraints)
		return append(dst, '}')
	case r.PcClose != nil:
		p := r.PcClose
		dst = append(dst, `{"pcClose":{"id":`...)
		dst = appendJSONString(dst, p.ID)
		dst = append(dst, `,"type":`...)
		dst = appendJSONString(dst, p.Type)
		if p.SubType != "" {
			dst = append(dst, `,"subType":`...)
			dst = appendJSONString(dst, p.SubType)
		}
		dst = append(dst, `,"data":`...)
		dst = appendJSONString(dst, p.Data)
		if p.Equiv != "" {
			dst = append(dst, `,"equiv":`...)
			dst = appendJSONString(dst, p.Equiv)
		}
		return append(dst, "}}"...)
	case r.Sub != nil:
		dst = append(dst, `{"sub":{"id":`...)
		dst = appendJSONString(dst, r.Sub.ID)
		dst = append(dst, `,"ref":`...)
		dst = appendJSONString(dst, r.Sub.Ref)
		dst = append(dst, `,"equiv":`...)
		dst = appendJSONString(dst, r.Sub.Equiv)
		return append(dst, "}}"...)
	case r.Plural != nil:
		dst = append(dst, `{"plural":{"pivot":`...)
		dst = appendJSONString(dst, r.Plural.Pivot)
		dst = append(dst, `,"forms":`...)
		if r.Plural.Forms == nil {
			dst = append(dst, "null"...)
		} else {
			keys := make([]string, 0, len(r.Plural.Forms))
			for k := range r.Plural.Forms {
				keys = append(keys, string(k))
			}
			slices.Sort(keys)
			dst = append(dst, '{')
			for i, k := range keys {
				if i > 0 {
					dst = append(dst, ',')
				}
				dst = appendJSONString(dst, k)
				dst = append(dst, ':')
				dst = appendRunsArray(dst, r.Plural.Forms[PluralForm(k)])
			}
			dst = append(dst, '}')
		}
		return append(dst, "}}"...)
	case r.Select != nil:
		dst = append(dst, `{"select":{"pivot":`...)
		dst = appendJSONString(dst, r.Select.Pivot)
		dst = append(dst, `,"cases":`...)
		if r.Select.Cases == nil {
			dst = append(dst, "null"...)
		} else {
			keys := make([]string, 0, len(r.Select.Cases))
			for k := range r.Select.Cases {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			dst = append(dst, '{')
			for i, k := range keys {
				if i > 0 {
					dst = append(dst, ',')
				}
				dst = appendJSONString(dst, k)
				dst = append(dst, ':')
				dst = appendRunsArray(dst, r.Select.Cases[k])
			}
			dst = append(dst, '}')
		}
		return append(dst, "}}"...)
	}
	return dst
}

// appendRunsArray appends a branch's runs as Run.MarshalJSON writes a slice
// field: null for a nil slice.
func appendRunsArray(dst []byte, runs []Run) []byte {
	if runs == nil {
		return append(dst, "null"...)
	}
	dst = append(dst, '[')
	for i, r := range runs {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendValidRun(dst, r)
	}
	return append(dst, ']')
}

func appendCodeJSON(dst []byte, id, typ, subType, data, equiv, disp string, attrs map[string]string, c *RunConstraints) []byte {
	dst = append(dst, `{"id":`...)
	dst = appendJSONString(dst, id)
	dst = append(dst, `,"type":`...)
	dst = appendJSONString(dst, typ)
	if subType != "" {
		dst = append(dst, `,"subType":`...)
		dst = appendJSONString(dst, subType)
	}
	dst = append(dst, `,"data":`...)
	dst = appendJSONString(dst, data)
	dst = append(dst, `,"equiv":`...)
	dst = appendJSONString(dst, equiv)
	if disp != "" {
		dst = append(dst, `,"disp":`...)
		dst = appendJSONString(dst, disp)
	}
	if len(attrs) > 0 {
		keys := make([]string, 0, len(attrs))
		for k := range attrs {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		dst = append(dst, `,"attrs":{`...)
		for i, k := range keys {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendJSONString(dst, k)
			dst = append(dst, ':')
			dst = appendJSONString(dst, attrs[k])
		}
		dst = append(dst, '}')
	}
	if c != nil {
		dst = append(dst, `,"constraints":{"deletable":`...)
		dst = strconv.AppendBool(dst, c.Deletable)
		dst = append(dst, `,"cloneable":`...)
		dst = strconv.AppendBool(dst, c.Cloneable)
		dst = append(dst, `,"reorderable":`...)
		dst = strconv.AppendBool(dst, c.Reorderable)
		dst = append(dst, '}')
	}
	return append(dst, '}')
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON string the way encoding/json writes it
// with HTML escaping off: quote and backslash escaped, \b \f \n \r \t by name,
// other control characters as \u00XX, U+2028 and U+2029 escaped, and each byte
// of invalid UTF-8 replaced by U+FFFD.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch c {
			case '"', '\\':
				dst = append(dst, '\\', c)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, "�"...)
			i++
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}
