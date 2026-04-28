//go:build parity

package parity

import (
	"encoding/json"
	"errors"
	"os"
)

// bridgeBinaryFromManifest reads a kapi plugin manifest and returns the
// declared `binary` field, the relative path within the plugin dir to
// the executable launcher.
func bridgeBinaryFromManifest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var m struct {
		Binary string `json:"binary"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	if m.Binary == "" {
		return "", errors.New("manifest binary field is empty")
	}
	return m.Binary, nil
}
