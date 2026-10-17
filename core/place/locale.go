package place

import "strings"

// OfLocale is the place a locale tag names: the subdivision its `-u-sd-` key
// names ("nb-NO-u-sd-no03" is no-03), else the country or M49 region its
// region subtag names ("nb-NO" is no, "es-419" is
// latin-america-and-the-caribbean). A tag with neither, or one naming a place
// the built-in gazetteer does not hold, names none.
func OfLocale(tag string) string {
	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })
	b := Builtin()
	region := ""
	for i := 1; i < len(parts); i++ {
		part := strings.ToLower(parts[i])
		if len(part) == 1 {
			if part == "x" {
				break
			}
			if part == "u" {
				if sd := subdivisionKey(parts[i+1:]); sd != "" {
					if b.Has(sd) {
						return sd
					}
					return ""
				}
			}
			// Every later subtag belongs to an extension, never the region.
			for i+1 < len(parts) && len(parts[i+1]) > 1 {
				i++
			}
			continue
		}
		if region != "" {
			continue
		}
		switch {
		case len(part) == 2 && isAlpha(part):
			if p, ok := b.base.places[part]; ok && p.Type == TypeCountry {
				region = part
			} else {
				return ""
			}
		case len(part) == 3 && isDigits(part):
			region = b.base.m49[part]
			if region == "" {
				return ""
			}
		}
	}
	return region
}

// subdivisionKey reads the `sd` key of a `-u-` extension's subtags, as a place
// id: "no03" is "no-03". It is empty when the extension has no such key.
func subdivisionKey(subtags []string) string {
	for i := range subtags {
		s := strings.ToLower(subtags[i])
		if len(s) == 1 {
			return ""
		}
		if s == "sd" && i+1 < len(subtags) {
			v := strings.ToLower(subtags[i+1])
			if len(v) < 3 || len(v) > 6 || !isAlpha(v[:2]) {
				return ""
			}
			return v[:2] + "-" + v[2:]
		}
	}
	return ""
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
