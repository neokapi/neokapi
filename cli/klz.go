package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz"
	"github.com/spf13/cobra"
)

// NewKLZCmd creates the `klz` command group — operate on .klz
// archives per RFC 0001. Subcommands: inspect, verify, diff,
// extract, pack, merge, annotate, orphans, annotations. Every
// subcommand is stateless and takes paths on the command line.
func (a *App) NewKLZCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "klz",
		Short:   "Operate on .klz localization archives (RFC 0001)",
		GroupID: "management",
		Long: `Inspect, verify, pack, extract, diff, merge, and annotate
Kapi Localization arcHive (.klz) files defined by RFC 0001. The commands
take archive paths on the command line — no project directory, no
session state.`,
	}
	cmd.AddCommand(a.newKLZInspectCmd())
	cmd.AddCommand(a.newKLZVerifyCmd())
	cmd.AddCommand(a.newKLZExtractCmd())
	cmd.AddCommand(a.newKLZPackCmd())
	cmd.AddCommand(a.newKLZDiffCmd())
	cmd.AddCommand(a.newKLZMergeCmd())
	cmd.AddCommand(a.newKLZAnnotationsCmd())
	cmd.AddCommand(a.newKLZAnnotateCmd())
	cmd.AddCommand(a.newKLZOrphansCmd())
	return cmd
}

// NewCacheCmd creates the `cache` command group — administer the
// runtime acceleration cache referenced by RFC 0001. In Phase 1 the
// commands cover info / path / clear; warm and gc surface Phase-4
// stubs that explain why the cache layer isn't built yet.
func (a *App) NewCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cache",
		Short:   "Administer the runtime acceleration cache",
		GroupID: "management",
		Long:    `Manage the content-addressed SQLite cache core/klz uses to accelerate random-access queries on .klz archives.`,
	}
	cmd.AddCommand(a.newCacheInfoCmd())
	cmd.AddCommand(a.newCachePathCmd())
	cmd.AddCommand(a.newCacheWarmCmd())
	cmd.AddCommand(a.newCacheGCCmd())
	cmd.AddCommand(a.newCacheClearCmd())
	return cmd
}

// ───────── klz inspect ─────────

func (a *App) newKLZInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <file.klz>",
		Short: "Dump manifest + document list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()

			m := r.Manifest()
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Archive:        %s\n", args[0])
			fmt.Fprintf(out, "Format:         %s\n", m.KapiLocalizationFormat)
			fmt.Fprintf(out, "Generator:      %s@%s\n", m.Generator.ID, m.Generator.Version)
			fmt.Fprintf(out, "Project:        %s (source=%s)\n", m.Project.ID, m.Project.SourceLocale)
			fmt.Fprintf(out, "Target locales: %s\n", strings.Join(m.Project.TargetLocales, ", "))
			fmt.Fprintf(out, "Manifest hash:  %s\n", r.ManifestHash())
			fmt.Fprintln(out, "Parts:")

			byRole := make(map[klz.PartRole][]klz.ManifestPartInfo)
			for _, p := range m.Parts {
				byRole[p.Role] = append(byRole[p.Role], p)
			}
			roles := make([]klz.PartRole, 0, len(byRole))
			for r := range byRole {
				roles = append(roles, r)
			}
			sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
			for _, role := range roles {
				fmt.Fprintf(out, "  %s:\n", role)
				for _, p := range byRole[role] {
					fmt.Fprintf(out, "    %s  sha256=%s  size=%d\n", p.Path, p.SHA256, p.Size)
				}
			}

			docs, err := r.Documents()
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "Documents:")
			for _, f := range docs {
				for _, d := range f.Documents {
					fmt.Fprintf(out, "  %s (%d blocks)\n", d.Path, len(d.Blocks))
				}
			}
			return nil
		},
	}
}

// ───────── klz verify ─────────

func (a *App) newKLZVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <file.klz>",
		Short: "Integrity + validator checks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()

			problems := r.VerifyAll()
			docs, docErr := r.Documents()
			var blockErrs []klf.ValidationError
			if docErr == nil {
				for _, f := range docs {
					for _, d := range f.Documents {
						for i := range d.Blocks {
							blockErrs = append(blockErrs, klf.ValidateBlock(&d.Blocks[i])...)
						}
					}
				}
			}
			out := cmd.OutOrStdout()
			if len(problems) == 0 && len(blockErrs) == 0 && docErr == nil {
				fmt.Fprintln(out, "OK: archive verified clean.")
				return nil
			}
			if docErr != nil {
				fmt.Fprintf(out, "error reading documents: %s\n", docErr)
			}
			for _, p := range problems {
				fmt.Fprintf(out, "archive: %s: %s — %s\n", p.Kind, p.Path, p.Message)
			}
			for _, e := range blockErrs {
				fmt.Fprintf(out, "block: %s: %s — %s\n", e.Kind, e.BlockID, e.Message)
			}
			return fmt.Errorf("verification failed: %d problem(s)", len(problems)+len(blockErrs))
		},
	}
}

// ───────── klz extract ─────────

func (a *App) newKLZExtractCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "extract <file.klz>",
		Short: "Unpack archive parts to a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				return fmt.Errorf("--out is required")
			}
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			// Write manifest first.
			manifestBytes, err := klz.MarshalManifest(r.Manifest())
			if err != nil {
				return err
			}
			if err := writeFileAtomic(filepath.Join(outDir, klz.ManifestPath), manifestBytes); err != nil {
				return err
			}
			for _, p := range r.Manifest().Parts {
				data, err := r.ReadPart(p.Path)
				if err != nil {
					return err
				}
				dest := filepath.Join(outDir, filepath.FromSlash(p.Path))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				if err := writeFileAtomic(dest, data); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "extracted %d parts to %s\n", len(r.Manifest().Parts), outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "output directory")
	return cmd
}

// ───────── klz pack ─────────

func (a *App) newKLZPackCmd() *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Pack a directory into a .klz archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outPath == "" {
				return fmt.Errorf("--out is required")
			}
			dir := args[0]
			manifestPath := filepath.Join(dir, klz.ManifestPath)
			manifestData, err := os.ReadFile(manifestPath)
			if err != nil {
				return fmt.Errorf("read manifest.json: %w", err)
			}
			manifest, err := klz.UnmarshalManifest(manifestData)
			if err != nil {
				return err
			}
			w := klz.NewWriter(klz.WriterOptions{
				Generator: manifest.Generator,
				Project:   manifest.Project,
				Created:   manifest.Created,
			})
			for _, p := range manifest.Parts {
				data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p.Path)))
				if err != nil {
					return fmt.Errorf("read part %s: %w", p.Path, err)
				}
				if err := addPartByRole(w, p.Path, data, p.Role, p.Attributes); err != nil {
					return err
				}
			}
			out, err := os.Create(outPath)
			if err != nil {
				return err
			}
			defer out.Close()
			if _, err := w.Write(out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "packed %d parts to %s\n", len(manifest.Parts), outPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "output .klz file")
	return cmd
}

func addPartByRole(w *klz.Writer, partPath string, data []byte, role klz.PartRole, attrs map[string]any) error {
	switch role {
	case klz.RoleDocument:
		return w.AddDocumentBytes(partPath, data, attrs)
	case klz.RoleTarget:
		// Reuse AddDocumentBytes semantics: both are .klf payloads;
		// the role stamp is what sets them apart in the manifest.
		// For targets we route via a dedicated typed wrapper to
		// keep intent visible in calling code.
		file, err := klf.Unmarshal(data)
		if err != nil {
			return err
		}
		return w.AddTarget(partPath, file, attrs)
	case klz.RoleSkeleton:
		return w.AddSkeleton(partPath, data, attrs)
	case klz.RoleVocabulary:
		return w.AddVocabulary(partPath, data, attrs)
	case klz.RoleAnnotation:
		file, err := klf.DecodeAnnotationFile(bytes.NewReader(data))
		if err != nil {
			return err
		}
		return w.AddAnnotationFile(partPath, file, attrs)
	case klz.RoleMeta:
		if partPath == "meta.json" {
			return w.AddMeta(data)
		}
		return w.AddAsset(partPath, data, attrs)
	default:
		return w.AddAsset(partPath, data, attrs)
	}
}

// ───────── klz diff ─────────

func (a *App) newKLZDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <a.klz> <b.klz>",
		Short: "Per-block diff of two archives",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ra, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer ra.Close()
			rb, err := klz.Open(args[1])
			if err != nil {
				return err
			}
			defer rb.Close()

			a := collectBlocks(ra)
			b := collectBlocks(rb)
			out := cmd.OutOrStdout()

			ids := make(map[string]bool, len(a)+len(b))
			for id := range a {
				ids[id] = true
			}
			for id := range b {
				ids[id] = true
			}
			sorted := make([]string, 0, len(ids))
			for id := range ids {
				sorted = append(sorted, id)
			}
			sort.Strings(sorted)

			added, removed, changed := 0, 0, 0
			for _, id := range sorted {
				ab, okA := a[id]
				bb, okB := b[id]
				switch {
				case !okA:
					fmt.Fprintf(out, "+ %s\n", id)
					added++
				case !okB:
					fmt.Fprintf(out, "- %s\n", id)
					removed++
				default:
					if ab.Hash != bb.Hash || !runsEqual(ab.Source, bb.Source) {
						fmt.Fprintf(out, "~ %s\n", id)
						changed++
					}
				}
			}
			fmt.Fprintf(out, "+%d -%d ~%d\n", added, removed, changed)
			return nil
		},
	}
}

func collectBlocks(r *klz.Reader) map[string]*klf.Block {
	out := make(map[string]*klf.Block)
	docs, err := r.Documents()
	if err != nil {
		return out
	}
	for _, f := range docs {
		for _, d := range f.Documents {
			for i := range d.Blocks {
				out[d.Blocks[i].ID] = &d.Blocks[i]
			}
		}
	}
	return out
}

func runsEqual(a, b []klf.Run) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

// ───────── klz merge ─────────

func (a *App) newKLZMergeCmd() *cobra.Command {
	var locale, outDir string
	cmd := &cobra.Command{
		Use:   "merge <file.klz>",
		Short: "Route to the extractor's Merge() for a given locale",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if locale == "" {
				return fmt.Errorf("--locale is required")
			}
			if outDir == "" {
				return fmt.Errorf("--out is required")
			}
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()
			gen := r.Manifest().Generator.ID
			// Phase 1 has no Go-side Extractor registry yet; merge
			// intentionally surfaces this as a deferred operation
			// and prints the generator id so tooling around kapi
			// (CI jobs, external extractors) can route the work.
			return fmt.Errorf("klz merge is delegated to extractor %q which is not registered in this build; see RFC 0001 §Extractor interface (phase 1)", gen)
		},
	}
	cmd.Flags().StringVar(&locale, "locale", "", "target locale to merge (e.g. de)")
	cmd.Flags().StringVar(&outDir, "out", "", "output directory")
	return cmd
}

// ───────── klz annotations ─────────

func (a *App) newKLZAnnotationsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "annotations <file.klz>",
		Short: "List annotation sidecars + producer namespaces",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()
			files, err := r.AnnotationFiles()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(files) == 0 {
				fmt.Fprintln(out, "(no annotations)")
				return nil
			}
			for _, f := range files {
				fmt.Fprintf(out, "%s: type=%q producer=%s@%s records=%d targetArchive=%s\n",
					f.Path, f.File.Header.AnnotationType,
					f.File.Header.Producer.ID, f.File.Header.Producer.Version,
					len(f.File.Annotations), f.File.Header.TargetArchive)
			}
			return nil
		},
	}
}

// ───────── klz annotate ─────────

func (a *App) newKLZAnnotateCmd() *cobra.Command {
	var inAnnotation, outPath, namespace string
	cmd := &cobra.Command{
		Use:   "annotate <file.klz>",
		Short: "Merge an annotation sidecar into an archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if inAnnotation == "" {
				return fmt.Errorf("--in is required")
			}
			if outPath == "" {
				return fmt.Errorf("--out is required")
			}
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()

			annBytes, err := os.ReadFile(inAnnotation)
			if err != nil {
				return err
			}
			annFile, err := klf.DecodeAnnotationFile(bytes.NewReader(annBytes))
			if err != nil {
				return err
			}

			// Copy every existing part into a fresh archive, then
			// add the annotation sidecar under the given namespace.
			w := klz.NewWriter(klz.WriterOptions{
				Generator: r.Manifest().Generator,
				Project:   r.Manifest().Project,
				Created:   r.Manifest().Created,
			})
			for _, p := range r.Manifest().Parts {
				data, err := r.ReadPart(p.Path)
				if err != nil {
					return err
				}
				if err := addPartByRole(w, p.Path, data, p.Role, p.Attributes); err != nil {
					return err
				}
			}
			ns := namespace
			if ns == "" {
				ns = annFile.Header.AnnotationType
			}
			safe := strings.ReplaceAll(ns, "/", "-")
			safe = strings.TrimPrefix(safe, "@")
			partPath := "annotations/" + safe + ".klfl"
			if err := w.AddAnnotationFile(partPath, annFile, nil); err != nil {
				return err
			}
			out, err := os.Create(outPath)
			if err != nil {
				return err
			}
			defer out.Close()
			if _, err := w.Write(out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s with %s\n", outPath, partPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&inAnnotation, "in", "", "path to a .klfl annotation file")
	cmd.Flags().StringVar(&outPath, "out", "", "output .klz path")
	cmd.Flags().StringVar(&namespace, "namespace", "", "producer namespace (defaults to header.annotationType)")
	return cmd
}

// ───────── klz orphans ─────────

func (a *App) newKLZOrphansCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "orphans <file.klz>",
		Short: "Resolve every annotation anchor; flag stale records",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()
			files, err := r.AnnotationFiles()
			if err != nil {
				return err
			}
			blocks := collectBlocks(r)
			out := cmd.OutOrStdout()

			total, orphans := 0, 0
			for _, f := range files {
				for _, ann := range f.File.Annotations {
					total++
					b, ok := blocks[ann.Anchor.Block]
					if !ok {
						fmt.Fprintf(out, "%s: %s — block-not-found\n", f.Path, ann.ID)
						orphans++
						continue
					}
					if err := klf.ValidateAnchor(b, ann); err != nil {
						fmt.Fprintf(out, "%s: %s — %s: %s\n", f.Path, ann.ID, err.Reason, err.Message)
						orphans++
					}
				}
			}
			fmt.Fprintf(out, "checked %d annotations, %d orphans\n", total, orphans)
			if orphans > 0 {
				return fmt.Errorf("%d orphaned annotations", orphans)
			}
			return nil
		},
	}
}

// ───────── cache subcommands ─────────

func (a *App) newCacheInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show cache location, entry count, and total size",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := klz.CacheRoot()
			fmt.Fprintf(cmd.OutOrStdout(), "Cache root: %s\n", root)
			count, size := cacheStats(root)
			fmt.Fprintf(cmd.OutOrStdout(), "Entries:    %d\n", count)
			fmt.Fprintf(cmd.OutOrStdout(), "Total size: %d bytes\n", size)
			return nil
		},
	}
}

func (a *App) newCachePathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <file.klz>",
		Short: "Print the cache directory for a given .klz",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()
			hash := r.ManifestHash()
			// Sanity: ensure the hash decodes cleanly before we use
			// it as a directory name. Any write to the reader that
			// preserves the raw manifest bytes yields a hex digest.
			if _, err := hex.DecodeString(hash); err != nil {
				return fmt.Errorf("invalid manifest hash %q: %w", hash, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), klz.CacheEntryDir(hash))
			return nil
		},
	}
}

func (a *App) newCacheWarmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "warm <file.klz>",
		Short: "Pre-build the cache entry for an archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := klz.Open(args[0])
			if err != nil {
				return err
			}
			defer r.Close()
			// Phase 1 surfaces WarmCache() which returns a deferred
			// error explaining that the cache layer lands in
			// Phase 4. The CLI wiring stays stable across that
			// bump.
			return r.WarmCache(context.Background())
		},
	}
}

func (a *App) newCacheGCCmd() *cobra.Command {
	var maxSize string
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "LRU-evict cache entries to a size cap",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("cache gc requires the klzcache build tag (phase 4, see RFC 0001); cap=%q", maxSize)
		},
	}
	cmd.Flags().StringVar(&maxSize, "max-size", "2GB", "target size cap")
	return cmd
}

func (a *App) newCacheClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove every cache entry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := klz.CacheRoot()
			// os.RemoveAll is safe when the path doesn't exist.
			if err := os.RemoveAll(root); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cleared cache at %s\n", root)
			return nil
		},
	}
}

func cacheStats(root string) (count int, size int64) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if path != root && info.Name()[0] != '.' {
				count++
			}
			return nil
		}
		size += info.Size()
		return nil
	})
	return count, size
}

// writeFileAtomic writes data to path via a temp file + rename.
// Used by klz extract to avoid partial writes if the caller
// interrupts mid-extract.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// discardWriter is a tiny helper used by some of the klz commands
// when they want to compute work without emitting output (e.g.
// verify + exit-code branch without cluttering a shared buffer).
var discardWriter io.Writer = io.Discard
