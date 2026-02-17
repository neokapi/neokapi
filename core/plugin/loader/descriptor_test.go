package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBridgeDescriptor(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		jarFile bool // whether to create a dummy JAR file
		wantErr string
	}{
		{
			name: "valid with all fields",
			json: `{
				"name": "okapi",
				"type": "bridge",
				"jar": "bridge.jar",
				"java": "/usr/bin/java",
				"jvm_args": ["-Xmx512m"],
				"startup_timeout": "10s",
				"command_timeout": "30s"
			}`,
			jarFile: true,
		},
		{
			name: "valid with defaults",
			json: `{
				"name": "okapi",
				"type": "bridge",
				"jar": "bridge.jar"
			}`,
			jarFile: true,
		},
		{
			name: "invalid type",
			json: `{
				"name": "okapi",
				"type": "binary",
				"jar": "bridge.jar"
			}`,
			jarFile: true,
			wantErr: `type must be "bridge"`,
		},
		{
			name: "missing jar",
			json: `{
				"name": "okapi",
				"type": "bridge"
			}`,
			wantErr: "jar field is required",
		},
		{
			name: "missing name",
			json: `{
				"type": "bridge",
				"jar": "bridge.jar"
			}`,
			jarFile: true,
			wantErr: "name field is required",
		},
		{
			name: "jar not found on disk",
			json: `{
				"name": "okapi",
				"type": "bridge",
				"jar": "nonexistent.jar"
			}`,
			wantErr: "jar file not found",
		},
		{
			name: "bad startup timeout",
			json: `{
				"name": "okapi",
				"type": "bridge",
				"jar": "bridge.jar",
				"startup_timeout": "notaduration"
			}`,
			jarFile: true,
			wantErr: "invalid startup_timeout",
		},
		{
			name: "bad command timeout",
			json: `{
				"name": "okapi",
				"type": "bridge",
				"jar": "bridge.jar",
				"command_timeout": "xyz"
			}`,
			jarFile: true,
			wantErr: "invalid command_timeout",
		},
		{
			name:    "invalid json",
			json:    `{broken`,
			wantErr: "parsing descriptor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			if tt.jarFile {
				err := os.WriteFile(filepath.Join(dir, "bridge.jar"), []byte("fake"), 0644)
				require.NoError(t, err)
			}

			descPath := filepath.Join(dir, "test.bridge.json")
			err := os.WriteFile(descPath, []byte(tt.json), 0644)
			require.NoError(t, err)

			parsed, err := ParseBridgeDescriptor(descPath, dir)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, parsed)
			assert.Equal(t, "okapi", parsed.Name)
			assert.Equal(t, "bridge", parsed.Type)
			assert.Equal(t, descPath, parsed.SourcePath)
			assert.FileExists(t, parsed.ResolvedJARPath)
			assert.True(t, parsed.ResolvedStartupTimeout > 0)
			assert.True(t, parsed.ResolvedCommandTimeout > 0)
		})
	}
}

func TestParseBridgeDescriptorDefaults(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "bridge.jar"), []byte("fake"), 0644)
	require.NoError(t, err)

	descPath := filepath.Join(dir, "test.bridge.json")
	err = os.WriteFile(descPath, []byte(`{
		"name": "okapi",
		"type": "bridge",
		"jar": "bridge.jar"
	}`), 0644)
	require.NoError(t, err)

	parsed, err := ParseBridgeDescriptor(descPath, dir)
	require.NoError(t, err)

	assert.Equal(t, "java", parsed.Java)
	assert.Equal(t, "30s", parsed.StartupTimeout)
	assert.Equal(t, "60s", parsed.CommandTimeout)
}

func TestParseBridgeDescriptorAllFields(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "my.jar"), []byte("fake"), 0644)
	require.NoError(t, err)

	descPath := filepath.Join(dir, "test.bridge.json")
	err = os.WriteFile(descPath, []byte(`{
		"name": "okapi",
		"type": "bridge",
		"jar": "my.jar",
		"java": "/opt/java/bin/java",
		"jvm_args": ["-Xmx1g", "-Dfoo=bar"],
		"startup_timeout": "15s",
		"command_timeout": "45s"
	}`), 0644)
	require.NoError(t, err)

	parsed, err := ParseBridgeDescriptor(descPath, dir)
	require.NoError(t, err)

	assert.Equal(t, "/opt/java/bin/java", parsed.Java)
	assert.Equal(t, []string{"-Xmx1g", "-Dfoo=bar"}, parsed.JVMArgs)
	assert.Equal(t, filepath.Join(dir, "my.jar"), parsed.ResolvedJARPath)
}

func TestParseBridgeDescriptorFileNotFound(t *testing.T) {
	_, err := ParseBridgeDescriptor("/nonexistent/file.json", "/tmp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading descriptor")
}
