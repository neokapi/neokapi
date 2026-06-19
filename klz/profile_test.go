package klz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSkeletonRoundTrip verifies a skeleton member round-trips with its data
// and identity metadata, and that the data is part of the content RootHash.
func TestSkeletonRoundTrip(t *testing.T) {
	skel := SkeletonDoc{
		Path: SkeletonDir + "app.json", SourcePath: "app.json",
		FormatID: "json", ContentHash: "sha256:abc", Content: BytesContent([]byte("SKELDATA")),
	}
	pkg := &Package{
		Skeletons: []SkeletonDoc{skel},
		Sources: []SourceIdentity{{
			SourcePath: "app.json", FormatID: "json", ContentHash: "sha256:abc",
			SkeletonPath: SkeletonDir + "app.json",
		}},
	}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	require.Len(t, got.Skeletons, 1)
	gotSkel, err := ReadAll(got.Skeletons[0].Content)
	require.NoError(t, err)
	assert.Equal(t, []byte("SKELDATA"), gotSkel)
	assert.Equal(t, "app.json", got.Skeletons[0].SourcePath)
	assert.Equal(t, "json", got.Skeletons[0].FormatID)
	assert.Equal(t, "sha256:abc", got.Skeletons[0].ContentHash)

	// Changing the skeleton bytes changes the content RootHash (it is content).
	pkg2 := &Package{
		Skeletons: []SkeletonDoc{{Path: skel.Path, SourcePath: skel.SourcePath, Content: BytesContent([]byte("DIFFERENT"))}},
		Sources:   pkg.Sources,
	}
	r1, err := pkg.RootHash()
	require.NoError(t, err)
	r2, err := pkg2.RootHash()
	require.NoError(t, err)
	assert.NotEqual(t, r1, r2, "skeleton data must be part of the RootHash")
}

// TestSourceRetentionOptIn verifies raw source rides only when embedded: a
// package with identity + skeleton but no Source member has no source/ member,
// while one with a Source member does.
func TestSourceRetentionOptIn(t *testing.T) {
	identityOnly := &Package{
		Skeletons: []SkeletonDoc{{Path: SkeletonDir + "a.json", SourcePath: "a.json", Content: BytesContent([]byte("S"))}},
		Sources:   []SourceIdentity{{SourcePath: "a.json", SkeletonPath: SkeletonDir + "a.json"}},
	}
	data, err := identityOnly.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Empty(t, got.Source, "default retention carries no raw source member")
	require.Len(t, got.Skeletons, 1)

	withSource := &Package{
		Source:    []SourceDoc{{Path: "source/a.json", Content: BytesContent([]byte("RAW"))}},
		Skeletons: identityOnly.Skeletons,
		Sources:   []SourceIdentity{{SourcePath: "a.json", SkeletonPath: SkeletonDir + "a.json", HasRawSource: true}},
	}
	data2, err := withSource.Marshal()
	require.NoError(t, err)
	got2, err := Unmarshal(data2)
	require.NoError(t, err)
	require.Len(t, got2.Source, 1)
	gotRaw, err := ReadAll(got2.Source[0].Content)
	require.NoError(t, err)
	assert.Equal(t, []byte("RAW"), gotRaw)
	require.Len(t, got2.Sources, 1)
	assert.True(t, got2.Sources[0].HasRawSource)
}

// TestKindRoundTrip verifies the kind discriminator round-trips and that the
// legacy kind alias maps to KindProject.
func TestKindRoundTrip(t *testing.T) {
	// Default → KindProject.
	pkg := &Package{Overlays: sampleOverlays()}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, KindProject, got.Kind)

	// Interchange profile + task metadata.
	inter := &Package{
		Kind:            KindInterchange,
		Overlays:        sampleOverlays(),
		InterchangeTask: &InterchangeTask{SourceLocale: "en", TargetLocale: "fr", SourceFiles: []string{"a.json"}},
	}
	data, err = inter.Marshal()
	require.NoError(t, err)
	got, err = Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, KindInterchange, got.Kind)
	require.NotNil(t, got.InterchangeTask)
	assert.Equal(t, "fr", got.InterchangeTask.TargetLocale)
	assert.Equal(t, []string{"a.json"}, got.InterchangeTask.SourceFiles)
}

// TestKindAliasAcceptsLegacy verifies a package written with the legacy Kind
// magic string still loads as KindProject.
func TestKindAliasAcceptsLegacy(t *testing.T) {
	pkg := &Package{Kind: Kind, Overlays: sampleOverlays()}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, KindProject, got.Kind)
}

// TestUnmarshalRejectsUnknownKind verifies a manifest with an unrecognized
// kind is rejected.
func TestUnmarshalRejectsUnknownKind(t *testing.T) {
	pkg := &Package{Kind: "bogus-kind", Overlays: sampleOverlays()}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	_, err = Unmarshal(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kind")
}
