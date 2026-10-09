package schemaversion_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/schemaversion"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// serializationFormat is one registered serialization format's reader,
// driven through its public entry point over a minimal payload whose
// schemaVersion the test chooses.
type serializationFormat struct {
	name    string
	current string
	// read decodes a minimal payload stamped with version and returns the
	// reader's error.
	read func(version string) error
}

// The four formats that carry a schemaVersion, each through the same entry
// point a user's file takes. Adding a format here is how its reader joins the
// shared contract; a format missing from this list has no cross-format
// guarantee.
var serializationFormats = []serializationFormat{
	{
		name:    "kbf",
		current: kbf.SchemaVersion,
		read: func(version string) error {
			_, err := kbf.Unmarshal(jsonEnvelope(kbf.Kind, version))
			return err
		},
	},
	{
		name:    "kmb",
		current: kmb.SchemaVersion,
		read: func(version string) error {
			_, err := kmb.Unmarshal(jsonEnvelope(kmb.Kind, version))
			return err
		},
	},
	{
		name:    "ktb",
		current: ktb.SchemaVersion,
		read: func(version string) error {
			_, err := ktb.Unmarshal(jsonEnvelope(ktb.Kind, version))
			return err
		},
	},
	{
		name:    "kpz",
		current: kpz.SchemaVersion,
		read: func(version string) error {
			_, err := kpz.Unmarshal(kpzWithVersion(version))
			return err
		},
	},
}

func jsonEnvelope(kind, version string) []byte {
	b, err := json.Marshal(map[string]any{"kind": kind, "schemaVersion": version})
	if err != nil {
		panic(err)
	}
	return b
}

// kpzWithVersion marshals an empty project package and restamps its
// manifest's schemaVersion, so the archive is otherwise exactly what the
// writer produces.
func kpzWithVersion(version string) []byte {
	pkg := &kpz.Package{Kind: kpz.KindProject}
	data, err := pkg.Marshal()
	if err != nil {
		panic(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			panic(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			panic(err)
		}
		if f.Name == kpz.ManifestPath {
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				panic(err)
			}
			m["schemaVersion"] = version
			if body, err = json.Marshal(m); err != nil {
				panic(err)
			}
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write(body); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return out.Bytes()
}

// TestEveryFormatAppliesTheSameVersionRule holds every serialization format
// to one accept/reject answer for the same schemaVersion input: an unknown
// MINOR within a known MAJOR is read, an unknown MAJOR is refused, and a
// string that is not MAJOR.MINOR is refused as invalid. The refusal is a
// *schemaversion.Error naming the format, the version found and the versions
// the build reads, so a caller can tell a stale file from a corrupt one.
func TestEveryFormatAppliesTheSameVersionRule(t *testing.T) {
	type verdict struct {
		accept  bool
		invalid bool
	}
	cases := []struct {
		name string
		// version is a template: {major} is the format's current major.
		version string
		want    verdict
	}{
		{"current version", "{current}", verdict{accept: true}},
		{"unknown minor of a known major", "{major}.99", verdict{accept: true}},
		{"unknown major", "9.0", verdict{}},
		{"major zero", "0.0", verdict{}},
		{"empty", "", verdict{invalid: true}},
		{"no dot", "{major}", verdict{invalid: true}},
		{"leading dot", ".5", verdict{invalid: true}},
		{"trailing dot", "{major}.", verdict{invalid: true}},
		{"letters after the dot", "{major}.abc", verdict{invalid: true}},
		{"letters only", "x", verdict{invalid: true}},
		{"three components", "{major}.0.0", verdict{invalid: true}},
		{"negative minor", "{major}.-1", verdict{invalid: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, f := range serializationFormats {
				major, ok := schemaversion.Major(f.current)
				require.True(t, ok, "%s declares a parseable SchemaVersion", f.name)
				version := strings.NewReplacer(
					"{current}", f.current,
					"{major}", fmt.Sprint(major),
				).Replace(tc.version)

				err := f.read(version)
				if tc.want.accept {
					assert.NoError(t, err, "%s must read schemaVersion %q", f.name, version)
					continue
				}
				require.Error(t, err, "%s must refuse schemaVersion %q", f.name, version)
				var sv *schemaversion.Error
				require.ErrorAs(t, err, &sv, "%s reports the refusal as a *schemaversion.Error: %v", f.name, err)
				assert.Equal(t, f.name, sv.Format, "the error names the format")
				assert.Equal(t, version, sv.Found, "the error carries the version found")
				assert.Equal(t, tc.want.invalid, sv.Invalid, "%s classifies %q", f.name, version)
				assert.Contains(t, sv.Supported, f.current, "the error lists the versions this build reads")
				if tc.want.invalid {
					assert.Contains(t, err.Error(), "invalid schemaVersion")
				} else {
					assert.Contains(t, err.Error(), "unsupported major schemaVersion")
					assert.Contains(t, err.Error(), "this build reads ")
					assert.Contains(t, err.Error(), f.current)
				}
			}
		})
	}
}

// TestErrorIsUnwrappable pins that a wrapped refusal still answers
// errors.As, which is what lets a caller distinguish a stale file from a
// corrupt one without parsing the message.
func TestErrorIsUnwrappable(t *testing.T) {
	_, err := schemaversion.Check("kbf", "9.0", "1.0", "2.0")
	wrapped := fmt.Errorf("open bundle: %w", err)
	var sv *schemaversion.Error
	require.True(t, errors.As(wrapped, &sv))
	assert.Equal(t, 9, sv.Major)
	assert.Equal(t, []string{"1.0", "2.0"}, sv.Supported)
	assert.Equal(t, "kbf: unsupported major schemaVersion 9 (this build reads 1.0 and 2.0)", err.Error())
}
