package pluginhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The rule per platform. On Windows it refuses nothing: Go reports a Windows
// directory's mode as 0777, so a mode test there would refuse every install,
// and access is an ACL the rule does not read. Elsewhere the owner must be the
// person or root, nobody else may write, and group write is accepted only for
// root's or an administrators' group.
func TestRefusePath(t *testing.T) {
	const me, other, adminGID, staffGID = 501, 502, 80, 20
	admin := func(gid int) bool { return gid == 0 || gid == adminGID }
	for _, tc := range []struct {
		name   string
		goos   string
		facts  fileFacts
		refuse string
	}{
		{"windows reports 0777 for every directory", "windows", fileFacts{mode: 0o777}, ""},
		{"windows, even another owner", "windows", fileFacts{mode: 0o777, uid: other, hasOwner: true}, ""},
		{"the person's own private directory", "darwin", fileFacts{mode: 0o700, uid: me, gid: staffGID, hasOwner: true}, ""},
		{"Homebrew: the person's, mode 755", "darwin", fileFacts{mode: 0o755, uid: me, gid: adminGID, hasOwner: true}, ""},
		{"root's, mode 755", "linux", fileFacts{mode: 0o755, uid: 0, gid: 0, hasOwner: true}, ""},
		{"group-writable by the admin group", "darwin", fileFacts{mode: 0o775, uid: 0, gid: adminGID, hasOwner: true}, ""},
		{"group-writable by root's group", "linux", fileFacts{mode: 0o775, uid: 0, gid: 0, hasOwner: true}, ""},
		{"another user's", "linux", fileFacts{mode: 0o755, uid: other, hasOwner: true}, "belongs to another user"},
		{"writable by everyone", "darwin", fileFacts{mode: 0o777, uid: me, gid: adminGID, hasOwner: true}, "writable by every user"},
		{"group-writable by an ordinary group", "darwin", fileFacts{mode: 0o775, uid: me, gid: staffGID, hasOwner: true}, "writable by its group"},
		{"group-writable with no group to judge", "plan9", fileFacts{mode: 0o775}, "writable by its group"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := refusePath(tc.goos, "/p", tc.facts, me, admin)
			if tc.refuse == "" {
				assert.NoError(t, err)
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.refuse)
			}
		})
	}
}
