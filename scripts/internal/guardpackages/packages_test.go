package guardpackages

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestListReportsDependencyBuildError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GOWORK", "off")
	if err := os.Mkdir("dependency", 0o755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string]string{
		"go.mod":               "module example.com/fixture\n\ngo 1.27.1\n",
		"app.go":               "package app\nimport _ \"example.com/fixture/dependency\"\n",
		"dependency/broken.go": "package dependency\nvar value = missingSymbol\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := List(".")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 || !strings.Contains(err.Error(), "undefined: missingSymbol") {
		t.Fatalf("expected the dependency's compiler error, got %v", err)
	}
}
