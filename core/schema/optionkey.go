package schema

import (
	"math"
	"reflect"
	"strconv"
)

// OptionKey is the scope segment for one enum option, keyed by its value so a
// reordered options list keeps every translation. A value that cannot name a
// key (an empty string, a struct, nil) falls back to the option's index.
//
// The builtins generator writes the metadata document with this function and
// the localizer reads it back with the same one, so a key the generator emits
// is a key a lookup finds. When the two disagreed, four xliff2 version labels
// were written under `options..label` and asked for under `options.0.label`,
// and they stayed English in every locale.
//
// The value is read by kind rather than by concrete type, because an option
// often carries a named string: the AI provider selector holds an
// aiprovider.ProviderID, and a type switch on `string` reads that as an
// unnameable value and keys all seven providers by index. The generator sees
// the same option through JSON, where a named string is a string, so it wrote
// `options.anthropic.label` while the runtime asked for `options.0.label`.
// Reading by kind makes the two agree, and it also makes a JSON round trip
// (which turns a Go int into a float64) spell a whole number the same way.
func OptionKey(value any, fallbackIndex int) string {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		if s := rv.String(); s != "" {
			return s
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if f == math.Trunc(f) && !math.IsInf(f, 0) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool())
	}
	return strconv.Itoa(fallbackIndex)
}
