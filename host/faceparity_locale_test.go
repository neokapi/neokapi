package host_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The locale leg of the face parity contract: a project declaring its locales
// in POSIX style answers in BCP-47, and answers the same at every face.
//
// The CLI leg is cli/faceparity_locale_test.go and the desktop leg is in the
// desktop backend, for the module reasons host/facetest describes.

func TestFaceParity_HostCanonicalizesDeclaredLocales(t *testing.T) {
	p := facetest.WritePosix(t)

	proj, err := project.Load(p.Recipe)
	require.NoError(t, err)

	// The recipe keeps what its author wrote.
	assert.Equal(t, "en_US", string(proj.Defaults.SourceLanguage))

	// The resolved runtime every face reads through does not.
	ctx := project.NewProjectContext(proj, p.Recipe)
	assert.Equal(t, facetest.PosixSourceLocale, ctx.SourceLocale)
	assert.Equal(t, []model.LocaleID{facetest.PosixTargetLocale}, ctx.TargetLocales)
}

func TestFaceParity_StatusReportsCanonicalLocales(t *testing.T) {
	p := facetest.WritePosix(t)

	a := &host.App{}
	a.InitRegistries()

	cmd := host.NewEnvCommand(t.Context(), "status")
	host.AddProjectFlag(cmd)
	host.AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", p.Recipe))
	require.NoError(t, cmd.Flags().Set("json", "true"))

	var buf capturingWriter
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, a.RunStatus(cmd, nil))

	var out host.StatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.NotEmpty(t, out.Locales, "the fixture declares a target")
	for _, lc := range out.Locales {
		assert.Equal(t, string(facetest.PosixTargetLocale), lc.Locale,
			"status reports the canonical tag for a POSIX-declared target")
	}
}
