package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Download progress redraws one line, which a terminal overwrites in place.
// Off a terminal every redraw lands as its own line, so a CI log collects one
// per read of the download. There the update reports its start and its result,
// and nothing in between.
func TestUpdateProgressIsBoundedOffATerminal(t *testing.T) {
	var out bytes.Buffer
	report := progressFunc(&out)
	for i := range 2000 {
		report(int64(i), 2000)
	}
	assert.Empty(t, out.String(), "a buffer is no terminal, so the redraw writes nothing")
}

// On a terminal the redraw is what a person watches, so it keeps writing the
// percentage over one line.
func TestUpdateProgressRedrawsOnATerminal(t *testing.T) {
	var out bytes.Buffer
	report := newProgress(&out, true)
	report(50, 100)
	report(100, 100)
	assert.Equal(t, "\rDownloading… 50%\rDownloading… 100%", out.String())
	assert.Equal(t, 2, strings.Count(out.String(), "\r"), "one line, redrawn")
}

// A download whose total the server did not report has no percentage to draw.
func TestUpdateProgressWithoutATotalDrawsNothing(t *testing.T) {
	var out bytes.Buffer
	newProgress(&out, true)(10, 0)
	assert.Empty(t, out.String())
}

// On Windows the release index has no entry for the build, which is installed
// and updated through winget. The notice names that command.
func TestUpdateWindowsWingetNotice(t *testing.T) {
	got := windowsWingetNotice("stable")
	assert.Contains(t, got, "installed and updated through winget")
	assert.Contains(t, got, "winget upgrade Neokapi.KapiCli")
}
