package syncv1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestDescriptorInitialises pins the raw descriptor against hand-editing.
//
// sync.pb.go embeds the FileDescriptorProto as a length-prefixed byte string.
// A hand-edited rename that changes an identifier without fixing its length
// prefix leaves the descriptor unparseable, and protoimpl panics during package
// init — so every binary importing this package dies at startup, with a build
// that compiled cleanly. That happened once (#1462) and was caught only after
// the fact. Touching a name below means regenerating with `make proto`, never
// editing sync.pb.go.
//
// Reaching protoreflect at all proves init() ran and the descriptor parsed.
func TestDescriptorInitialises(t *testing.T) {
	md := (&SyncMemoryEntry{}).ProtoReflect().Descriptor()
	require.Equal(t, protoreflect.FullName("neokapi.sync.v1.SyncMemoryEntry"), md.FullName())

	// Walk every message in the file so a corrupt prefix anywhere surfaces here
	// rather than at some importer's startup.
	fd := md.ParentFile()
	require.Equal(t, "core/proto/sync/v1/sync.proto", fd.Path())
	for i := range fd.Messages().Len() {
		m := fd.Messages().Get(i)
		assert.NotEmpty(t, m.Name(), "message %d has no name", i)
		for j := range m.Fields().Len() {
			assert.NotEmpty(t, m.Fields().Get(j).Name())
		}
	}
}

// TestMemoryVocabularyOnTheWire pins the wire names the memory rename settled
// on. These are protocol schema: the field names below are what protojson
// emits and what a peer parses, so a drive-by rename is a wire break and must
// fail here first.
func TestMemoryVocabularyOnTheWire(t *testing.T) {
	t.Run("SyncChunk.memory_entries", func(t *testing.T) {
		fd := (&SyncChunk{}).ProtoReflect().Descriptor().Fields().ByNumber(12)
		require.NotNil(t, fd)
		assert.Equal(t, protoreflect.Name("memory_entries"), fd.Name())
		assert.Equal(t, "memoryEntries", fd.JSONName())
		assert.Equal(t, protoreflect.FullName("neokapi.sync.v1.SyncMemoryEntry"), fd.Message().FullName())
	})

	t.Run("SyncPushInit.memory_hash", func(t *testing.T) {
		fd := (&SyncPushInit{}).ProtoReflect().Descriptor().Fields().ByNumber(7)
		require.NotNil(t, fd)
		assert.Equal(t, protoreflect.Name("memory_hash"), fd.Name())
	})

	t.Run("SyncPushInitResponse.memory_changed", func(t *testing.T) {
		fd := (&SyncPushInitResponse{}).ProtoReflect().Descriptor().Fields().ByNumber(7)
		require.NotNil(t, fd)
		assert.Equal(t, protoreflect.Name("memory_changed"), fd.Name())
	})
}

// TestMemoryEntryRoundTrips exercises the descriptor through both codecs, which
// is what actually dereferences the parsed field metadata.
func TestMemoryEntryRoundTrips(t *testing.T) {
	in := &SyncChunk{
		ContentType: "memory",
		RecordCount: 1,
		MemoryEntries: []*SyncMemoryEntry{{
			Id:           "e1",
			SourceLocale: "en",
			TargetLocale: "fr",
			SourceText:   "Hello",
			TargetText:   "Bonjour",
			Origin:       "human",
			Score:        1,
			ContentHash:  "h1",
		}},
	}

	wire, err := proto.Marshal(in)
	require.NoError(t, err)
	var out SyncChunk
	require.NoError(t, proto.Unmarshal(wire, &out))
	require.Len(t, out.GetMemoryEntries(), 1)
	assert.Equal(t, "Bonjour", out.GetMemoryEntries()[0].GetTargetText())

	js, err := protojson.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(js), "memoryEntries")
	assert.NotContains(t, string(js), "tmEntries")
}
