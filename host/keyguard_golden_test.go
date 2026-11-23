package host

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// TestRecordEntryIDIsStable pins the id of the content memory entry an
// absorbed record writes, over the source key recordSourceKey gives.
func TestRecordEntryIDIsStable(t *testing.T) {
	runs := []model.Run{{Text: &model.TextRun{Text: "Save the file"}}}
	assert.Equal(t, "Save the file\x00Save the file", recordSourceKey(runs))
	assert.Equal(t, "record:02ee1e6dfb0a691d331527ab", recordEntryID(recordSourceKey(runs), ""))
	assert.Equal(t, "record:63b6e17e494d15e7cc6e63ac", recordEntryID(recordSourceKey(runs), "p\x1fc\x1fdocs"))
}
