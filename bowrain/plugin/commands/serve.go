package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

var (
	servePort   int
	serveNoOpen bool
)

var serveCmd = &cobra.Command{
	Use:   "serve [project-dir]",
	Short: "Open a local dashboard",
	Long: `Open a local web dashboard for this project.

If no directory is given, the current directory is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath := "."
		if len(args) > 0 {
			projectPath = args[0]
		}

		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		proj, err := project.FindProject(absPath)
		if err != nil {
			return fmt.Errorf("no kapi project found in %s (run 'kapi init' first)", absPath)
		}

		addr := fmt.Sprintf("localhost:%d", servePort)
		url := "http://" + addr

		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/project", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"root":             proj.Root,
				"source_language":  proj.Recipe.Defaults.SourceLanguage,
				"target_languages": proj.Recipe.Defaults.TargetLanguages,
				"has_server":       proj.Recipe.HasServer(),
				"content":          len(proj.Recipe.Content),
			})
		})
		mux.HandleFunc("GET /api/flows", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			var flows []string
			entries, _ := os.ReadDir(proj.FlowsDirPath())
			for _, e := range entries {
				if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
					flows = append(flows, e.Name())
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"flows": flows})
		})
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			dirName := filepath.Base(proj.Root)
			fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>bowrain — %s</title></head><body>
<h1>bowrain project: %s</h1>
<p>Root: %s</p>
<p>Source language: %s</p>
<p>API: <a href="/api/project">/api/project</a> | <a href="/api/flows">/api/flows</a></p>
</body></html>`, dirName, dirName, proj.Root, proj.Recipe.Defaults.SourceLanguage)
		})

		fmt.Printf("Starting local project dashboard at %s\n", url)
		fmt.Printf("Project: %s\n", absPath)
		if !serveNoOpen {
			openBrowser(url)
		}
		fmt.Println("Press Ctrl+C to stop.")

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nShutting down...")
			os.Exit(0)
		}()

		return http.ListenAndServe(addr, mux)
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 3000, "Port to listen on")
	serveCmd.Flags().BoolVar(&serveNoOpen, "no-open", false, "Don't open browser automatically")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(serveCmd) })
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:noctx // open browser
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:noctx // open browser
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:noctx // open browser
	default:
		fmt.Printf("Open %s in your browser\n", url)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Open %s in your browser\n", url)
	}
}
