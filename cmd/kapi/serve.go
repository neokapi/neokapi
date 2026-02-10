package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/gokapi/gokapi/internal/server"
	"github.com/spf13/cobra"
)

var (
	servePort   int
	serveNoOpen bool
)

var serveCmd = &cobra.Command{
	Use:   "serve [project.kaz | project-dir]",
	Short: "Start a local project server with web UI",
	Long: `Start a lightweight web server for editing a single local project.
This is similar to 'jupyter notebook' — it serves a web UI on localhost
for the project, with no authentication required.

If a .kaz file is given, it is imported into a temporary store.
On exit, changes are exported back to the file.

If no argument is given, the current directory is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath := "."
		if len(args) > 0 {
			projectPath = args[0]
		}

		// Resolve to absolute path.
		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		// Determine store path. For .kaz files, use a temp database next to the file.
		storePath := filepath.Join(os.TempDir(), "gokapi-serve.db")
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			storePath = filepath.Join(absPath, ".gokapi.db")
		}

		cfg := server.LocalServerConfig()
		cfg.Port = servePort
		cfg.StorePath = storePath

		srv := server.NewServer(cfg)

		// If given a .kaz file, import it.
		isKAZ := filepath.Ext(absPath) == ".kaz"
		var projectID string
		if isKAZ {
			if srv.ContentStore != nil {
				f, err := os.Open(absPath)
				if err != nil {
					return fmt.Errorf("open KAZ file: %w", err)
				}
				defer f.Close()
				projectID, err = srv.ContentStore.ImportKAZ(cmd.Context(), f)
				if err != nil {
					return fmt.Errorf("import KAZ: %w", err)
				}
				log.Printf("Imported project %s from %s", projectID, absPath)
			}
		}

		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		url := fmt.Sprintf("http://localhost:%d", cfg.Port)

		fmt.Printf("Starting local project server at %s\n", url)
		fmt.Printf("Project: %s\n", absPath)
		if !serveNoOpen {
			openBrowser(url)
		}
		fmt.Println("Press Ctrl+C to stop.")

		// Handle graceful shutdown.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")

			// If we imported a .kaz file, export changes back.
			if isKAZ && srv.ContentStore != nil && projectID != "" {
				log.Printf("Saving changes back to %s", absPath)
				f, err := os.Create(absPath)
				if err != nil {
					log.Printf("ERROR: create export file: %v", err)
				} else {
					if err := srv.ContentStore.ExportKAZ(cmd.Context(), projectID, f); err != nil {
						log.Printf("ERROR: export KAZ: %v", err)
					} else {
						log.Printf("Saved project to %s", absPath)
					}
					f.Close()
				}
			}

			if srv.ContentStore != nil {
				srv.ContentStore.Close()
			}

			os.Exit(0)
		}()

		return srv.Start(addr)
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 3000, "Port to listen on")
	serveCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "Don't open browser automatically")
	rootCmd.AddCommand(serveCmd)
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		fmt.Printf("Open %s in your browser\n", url)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Open %s in your browser\n", url)
	}
}
