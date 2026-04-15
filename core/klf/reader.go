package klf

import (
	"encoding/json"
	"fmt"
	"io"
)

// Unmarshal decodes a .klf JSON payload into a File, returning an
// error if the payload's kind or major schema version is unknown.
// Unknown minor versions within the same major are accepted per the
// forward-compatibility contract in RFC 0001 §Versioning.
func Unmarshal(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("klf: decode: %w", err)
	}
	if err := checkEnvelope(&f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Decode streams a .klf JSON payload from an io.Reader.
func Decode(r io.Reader) (*File, error) {
	dec := json.NewDecoder(r)
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("klf: decode: %w", err)
	}
	if err := checkEnvelope(&f); err != nil {
		return nil, err
	}
	return &f, nil
}

func checkEnvelope(f *File) error {
	if f.Kind != Kind {
		return fmt.Errorf("klf: unexpected kind %q (want %q)", f.Kind, Kind)
	}
	major, _, ok := splitVersion(f.SchemaVersion)
	if !ok {
		return fmt.Errorf("klf: invalid schemaVersion %q", f.SchemaVersion)
	}
	wantMajor, _, _ := splitVersion(SchemaVersion)
	if major != wantMajor {
		return fmt.Errorf("klf: unsupported major schemaVersion %d (this build speaks %s)", major, SchemaVersion)
	}
	return nil
}

func splitVersion(v string) (major, minor int, ok bool) {
	// Tiny parser: MAJOR.MINOR, both non-negative.
	if v == "" {
		return 0, 0, false
	}
	dot := -1
	for i, r := range v {
		if r == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return 0, 0, false
	}
	majStr, minStr := v[:dot], v[dot+1:]
	for _, r := range majStr {
		if r < '0' || r > '9' {
			return 0, 0, false
		}
		major = major*10 + int(r-'0')
	}
	for _, r := range minStr {
		if r < '0' || r > '9' {
			return 0, 0, false
		}
		minor = minor*10 + int(r-'0')
	}
	return major, minor, true
}
