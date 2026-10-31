package source

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// The preview's read carries the scan's resolved keys onto the same blocks:
// by reader id when the id names a scanned block with the same name, by reader
// name when the two reads numbered the blocks differently, and not at all for
// a name more than one scanned block carries.
func TestCarryResolvedKeys(t *testing.T) {
	scanned := []*model.Block{
		{ID: "1", Name: "title", Key: "u-title"},
		{ID: "2", Name: "intro", Key: "u-intro"},
		{ID: "3", Name: "item", Key: "u-item-1"},
		{ID: "4", Name: "item", Key: "u-item-2"},
	}

	same := []*model.Block{{ID: "1", Name: "title"}, {ID: "2", Name: "intro"}}
	carryResolvedKeys(same, scanned)
	assert.Equal(t, "u-title", same[0].Key)
	assert.Equal(t, "u-intro", same[1].Key)

	// The fresh read numbers from 0: id 1 names "title" in the scan, and the
	// block carrying it here is "intro".
	shifted := []*model.Block{{ID: "0", Name: "title"}, {ID: "1", Name: "intro"}}
	carryResolvedKeys(shifted, scanned)
	assert.Equal(t, "u-title", shifted[0].Key, "matched by name when the id is unknown")
	assert.Equal(t, "u-intro", shifted[1].Key, "matched by name when the id names another block")

	ambiguous := []*model.Block{{ID: "9", Name: "item"}}
	carryResolvedKeys(ambiguous, scanned)
	assert.Empty(t, ambiguous[0].Key, "a name two scanned blocks carry matches neither")
}
