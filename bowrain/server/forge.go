package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/forge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// maxForgeWebhookBody bounds an inbound webhook payload. Push events are a few
// KB; a megabyte of headroom keeps large multi-commit pushes while refusing
// abuse.
const maxForgeWebhookBody = 1 << 20

// HandleForgeWebhook receives push webhooks from GitHub or GitLab for a
// persisted forge connector: POST /api/webhooks/forge/:configID.
//
// The route is unauthenticated (forges cannot log in) and verified instead by
// the connector's webhook secret — GitHub signs the body (X-Hub-Signature-256,
// HMAC-SHA256), GitLab echoes the secret (X-Gitlab-Token). A verified push to
// the connector's tracked branch re-ingests the source and publishes
// EventPushCompleted, which starts a convergence run under the project's
// normal on-push policy. Pushes to any other branch — including the
// connector's own delivery branch — are acknowledged and ignored, which is
// what keeps delivery pushes from re-triggering the loop.
func (s *Server) HandleForgeWebhook(c echo.Context) error {
	if s.ConnectorConfigStore == nil || s.Services == nil || s.Services.Connector == nil || s.EventBus == nil {
		return c.NoContent(http.StatusNotFound)
	}
	configID := c.Param("configID")

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxForgeWebhookBody))
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}

	cfg, ok := s.forgeConfigByID(c.Request().Context(), configID)
	if !ok {
		// Unknown id and wrong-type id are indistinguishable: no probing.
		return c.NoContent(http.StatusNotFound)
	}
	secret := cfg.Config["webhook_secret"]
	if secret == "" {
		// A connector without a webhook secret has webhooks disabled.
		return c.NoContent(http.StatusNotFound)
	}

	kind := forge.Kind(cfg.Config["forge"])
	if kind != forge.KindGitHub && kind != forge.KindGitLab {
		kind = forge.KindForRepo(cfg.Config["repo"])
	}
	verified := false
	switch kind {
	case forge.KindGitHub:
		verified = forge.VerifyGitHubSignature(secret, body, c.Request().Header.Get("X-Hub-Signature-256"))
	default:
		verified = forge.VerifyGitLabToken(secret, c.Request().Header.Get("X-Gitlab-Token"))
	}
	if !verified {
		return c.NoContent(http.StatusUnauthorized)
	}

	ev, ok := forge.ParsePushEvent(kind, body)
	if !ok {
		// Not a branch push (ping, tag, MR event, …): acknowledged, no action.
		return c.NoContent(http.StatusAccepted)
	}
	baseBranch := cfg.Config["branch"]
	if baseBranch == "" {
		baseBranch = "main"
	}
	if ev.Branch != baseBranch {
		return c.NoContent(http.StatusAccepted)
	}

	projectID := cfg.Config["project_id"]
	workspaceID := cfg.WorkspaceID
	connectorID := cfg.ID

	// Ingest + converge off the request: forges time webhooks out in seconds,
	// while a fetch clones/pulls a repository.
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request().Context()), 10*time.Minute)
		defer cancel()
		if _, err := s.Services.Connector.Fetch(ctx, workspaceID, connectorID, projectID, platconn.FetchOptions{}); err != nil {
			slog.Warn("forge webhook: fetch failed", "connector", connectorID, "project", projectID, "error", err)
			return
		}
		s.EventBus.Publish(platev.Event{
			ID:        id.New(),
			Type:      platev.EventPushCompleted,
			Source:    "forge-webhook",
			ProjectID: projectID,
			Timestamp: time.Now().UTC(),
			Data: map[string]string{
				"connector_id": connectorID,
				"repo":         ev.RepoPath,
				"branch":       ev.Branch,
			},
		})
	}()

	return c.NoContent(http.StatusAccepted)
}

// forgeConfigByID finds a persisted forge connector config by id across
// workspaces — the webhook route carries no workspace, and config ids are
// globally unique.
func (s *Server) forgeConfigByID(ctx context.Context, configID string) (bstore.ConnectorConfig, bool) {
	if configID == "" {
		return bstore.ConnectorConfig{}, false
	}
	configs, err := s.ConnectorConfigStore.ListAll(ctx)
	if err != nil {
		slog.Warn("forge webhook: config lookup failed", "error", err)
		return bstore.ConnectorConfig{}, false
	}
	for _, cfg := range configs {
		if cfg.ID == configID && cfg.Type == "forge" {
			return cfg, true
		}
	}
	return bstore.ConnectorConfig{}, false
}

// subscribeForgeDelivery wires the delivery tier: when a convergence run ends
// converged or parked, every forge connector bound to the project publishes
// the produced translations as a pull/merge request. Failed and canceled runs
// deliver nothing.
func (s *Server) subscribeForgeDelivery() {
	if s.EventBus == nil || s.Services == nil || s.Services.Connector == nil || s.ConnectorConfigStore == nil {
		return
	}
	s.EventBus.Subscribe(platev.EventConvergenceRunCompleted, func(ev platev.Event) {
		state := ev.Data["state"]
		if state != bstore.ConvergenceRunConverged && state != bstore.ConvergenceRunParked {
			return
		}
		go s.deliverToForges(context.WithoutCancel(context.Background()), ev)
	})
}

// deliverToForges materializes the project's translated content and hands it
// to each bound forge connector as per-locale target files.
func (s *Server) deliverToForges(ctx context.Context, ev platev.Event) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	configs, err := s.ConnectorConfigStore.ListAll(ctx)
	if err != nil {
		slog.Warn("forge delivery: config list failed", "error", err)
		return
	}
	var bound []bstore.ConnectorConfig
	for _, cfg := range configs {
		if cfg.Type == "forge" && cfg.Config["project_id"] == ev.ProjectID {
			bound = append(bound, cfg)
		}
	}
	if len(bound) == 0 {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
	if err != nil {
		slog.Warn("forge delivery: project lookup failed", "project", ev.ProjectID, "error", err)
		return
	}
	items, err := s.materializeDelivery(ctx, proj)
	if err != nil {
		slog.Warn("forge delivery: materialize failed", "project", ev.ProjectID, "error", err)
		return
	}
	if len(items) == 0 {
		return
	}

	title, bodyText := forgeDeliveryReport(ev, proj)
	for _, cfg := range bound {
		conn, err := s.Services.Connector.Connector(cfg.WorkspaceID, cfg.ID)
		if err != nil {
			slog.Warn("forge delivery: connector not live", "connector", cfg.ID, "error", err)
			continue
		}
		err = conn.Publish(ctx, items, platconn.PublishOptions{
			Message: fmt.Sprintf("chore: update translations (run %s)", ev.Data["run_id"]),
			Metadata: map[string]string{
				"pr_title": title,
				"pr_body":  bodyText,
			},
		})
		if err != nil {
			slog.Warn("forge delivery: publish failed", "connector", cfg.ID, "project", ev.ProjectID, "error", err)
			continue
		}
		slog.Info("forge delivery: delivered", "connector", cfg.ID, "project", ev.ProjectID, "run", ev.Data["run_id"])
	}
}

// materializeDelivery turns the project's stored blocks into per-locale target
// files: for every source item and every target language, a copy of the item's
// blocks with that locale's translation promoted to the source position (what
// a format writer emits), at the conventional target path. Items where a
// locale has no translations at all are skipped rather than delivered empty.
func (s *Server) materializeDelivery(ctx context.Context, proj *platstore.Project) ([]*platconn.ContentItem, error) {
	storeItems, err := s.ContentStore.ListItems(ctx, proj.ID, "main")
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	sourceLang := string(proj.DefaultSourceLanguage)

	var out []*platconn.ContentItem
	for _, item := range storeItems {
		blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: proj.ID, Stream: "main", ItemName: item.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("get blocks for %s: %w", item.Name, err)
		}
		for _, locale := range proj.TargetLanguages {
			var promoted []*model.Block
			for _, sb := range blocks {
				if sb.Block == nil || !sb.Block.HasTarget(locale) {
					continue
				}
				cp := *sb.Block
				cp.Source = sb.Block.TargetRuns(locale)
				promoted = append(promoted, &cp)
			}
			if len(promoted) == 0 {
				continue
			}
			out = append(out, &platconn.ContentItem{
				ID:     item.Name + ":" + string(locale),
				Name:   item.Name,
				Path:   targetPathFor(item.Name, sourceLang, string(locale)),
				Format: item.Format,
				Locale: locale,
				Blocks: promoted,
			})
		}
	}
	return out, nil
}

// targetPathFor derives the target file path for a locale from the source
// item's path by the locale-segment convention: a path segment equal to the
// source language becomes the target language (locales/en/app.json →
// locales/fr/app.json); else a filename stem equal to the source language is
// swapped (content/en.json → content/fr.json); else the locale is suffixed to
// the stem (app.json → app.fr.json).
func targetPathFor(sourcePath, sourceLang, lang string) string {
	if sourceLang != "" {
		segs := strings.Split(sourcePath, "/")
		for i, seg := range segs {
			if strings.EqualFold(seg, sourceLang) {
				segs[i] = lang
				return strings.Join(segs, "/")
			}
		}
		dir, file := path.Split(sourcePath)
		ext := path.Ext(file)
		stem := strings.TrimSuffix(file, ext)
		if strings.EqualFold(stem, sourceLang) {
			return dir + lang + ext
		}
	}
	dir, file := path.Split(sourcePath)
	ext := path.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	return dir + stem + "." + lang + ext
}

// forgeDeliveryReport renders the PR/MR title and body for a completed run.
func forgeDeliveryReport(ev platev.Event, proj *platstore.Project) (title, body string) {
	state := ev.Data["state"]
	title = "Update translations"
	if state == bstore.ConvergenceRunParked {
		title = "Update translations (partial — review needed)"
	}

	var b strings.Builder
	b.WriteString("<!-- bowrain-convergence-report -->\n")
	b.WriteString("## Convergence report\n\n")
	fmt.Fprintf(&b, "| | |\n|---|---|\n| Outcome | **%s** |\n| Passes | %s |\n| Project | %s |\n",
		state, ev.Data["passes"], proj.Name)
	b.WriteString("\nProduced by the Bowrain convergence loop. ")
	if state == bstore.ConvergenceRunParked {
		b.WriteString("Some work parked for a person — it is waiting in the project's review queue and will arrive in a later delivery once approved.")
	} else {
		b.WriteString("Every gated scope cleared its ship gate.")
	}
	return title, b.String()
}
