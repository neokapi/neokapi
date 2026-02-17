package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	kapiweb "github.com/gokapi/gokapi/bowrain/apps/kapi-web"
	"github.com/gokapi/gokapi/bowrain/server"
	"github.com/spf13/cobra"
)

var (
	servePort   int
	serveNoOpen bool
)

var serveCmd = &cobra.Command{
	Use:   "serve [project-dir]",
	Short: "Start a local project server with web UI",
	Long: `Start a lightweight web server for editing a single local project.
This is similar to 'jupyter notebook' — it serves a web UI on localhost
for the project, with no authentication required.

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

		// Look for .kapi/ project directory.
		kapiDir := filepath.Join(absPath, ".kapi")
		if _, err := os.Stat(kapiDir); os.IsNotExist(err) {
			return fmt.Errorf("no .kapi/ project found in %s (run 'kapi init' first)", absPath)
		}

		storePath := filepath.Join(kapiDir, "store.db")

		cfg := server.LocalServerConfig()
		cfg.Port = servePort
		cfg.StorePath = storePath
		cfg.DataDir = kapiDir

		srv := server.NewServer(cfg)

		// Serve embedded web UI.
		webFS, _ := fs.Sub(kapiweb.Assets, "dist")
		srv.WebUIFS = webFS

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
