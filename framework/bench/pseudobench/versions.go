package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// buildVersions checks out git tags and builds kapi binaries for each.
func buildVersions(repoRoot, outputDir string, tags []string) error {
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}

	// Save current branch/commit to restore later.
	origRef, err := runCommand("git", "-C", absRepo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	origRef = strings.TrimSpace(origRef)

	// Check for dirty working tree.
	status, err := runCommand("git", "-C", absRepo, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("working tree is dirty — commit or stash changes first")
	}

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		fmt.Printf("Building kapi %s...\n", tag)

		// Checkout tag
		if _, err := runCommand("git", "-C", absRepo, "checkout", tag); err != nil {
			fmt.Printf("  SKIP %s: checkout failed: %v\n", tag, err)
			continue
		}

		// Build kapi
		outputPath := filepath.Join(absOutput, fmt.Sprintf("kapi-%s", tag))
		buildCmd := exec.Command("go", "build", "-o", outputPath, "./cmd/kapi/")
		buildCmd.Dir = filepath.Join(absRepo, "kapi")
		buildCmd.Env = append(os.Environ(),
			fmt.Sprintf("GOWORK=%s", filepath.Join(absRepo, "go.work")),
		)
		buildCmd.Stdout = os.Stderr
		buildCmd.Stderr = os.Stderr

		if err := buildCmd.Run(); err != nil {
			fmt.Printf("  SKIP %s: build failed: %v\n", tag, err)
			continue
		}

		fmt.Printf("  Built %s\n", outputPath)
	}

	// Restore original branch
	fmt.Printf("Restoring %s...\n", origRef)
	if _, err := runCommand("git", "-C", absRepo, "checkout", origRef); err != nil {
		return fmt.Errorf("restore branch: %w", err)
	}

	fmt.Println("Done building versions.")
	return nil
}

