package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapText(t *testing.T) {
	t.Run("width 0 returns text unbroken", func(t *testing.T) {
		got := wrapText("a long description that should not wrap", 0)
		assert.Equal(t, []string{"a long description that should not wrap"}, got)
	})

	t.Run("wraps at spaces within width", func(t *testing.T) {
		got := wrapText("alpha beta gamma delta", 11)
		assert.Equal(t, []string{"alpha beta", "gamma delta"}, got)
		for _, line := range got {
			assert.LessOrEqual(t, len(line), 11)
		}
	})

	t.Run("a word longer than width is not broken", func(t *testing.T) {
		got := wrapText("supercalifragilistic short", 10)
		assert.Equal(t, []string{"supercalifragilistic", "short"}, got)
	})

	t.Run("empty string yields one empty line", func(t *testing.T) {
		assert.Equal(t, []string{""}, wrapText("", 20))
	})

	t.Run("reassembling preserves words", func(t *testing.T) {
		const desc = "Video demux dependency for kapi — bundles LGPL ffmpeg/ffprobe so the reader can extract audio"
		got := wrapText(desc, 30)
		assert.Equal(t, strings.Fields(desc), strings.Fields(strings.Join(got, " ")))
		for _, line := range got {
			assert.LessOrEqual(t, len([]rune(line)), 30, "line exceeds width: %q", line)
		}
	})
}
