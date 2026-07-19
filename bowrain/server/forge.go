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
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
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

	s.forgeIngest(c.Request().Context(), cfg, ev, "forge-webhook")
	return c.NoContent(http.StatusAccepted)
}

// forgeIngest hands a connector's source ingest off the calling request — a
// fetch clones/pulls a repository, which is far too slow for a webhook
// response (forges time webhooks out in seconds) or a bind response.
//
// The primary path is a **durable job** on the translation queue, executed by
// bowrain-worker: the previous fire-and-forget goroutine died with the server
// process (a DRAINING ECS task mid-deploy acked the webhook 202 and then lost
// the work until a manual redelivery — the production drill this fixes). The
// job row + broker message survive the process, and the worker's lease/retry/
// sweeper machinery delivers at-least-once; re-ingest is idempotent (see
// jobs.ForgeIngest).
//
// Deployments without the job system wired (desktop/in-memory, tests, dev
// without a broker) fall back to the same in-process goroutine as before,
// running the identical shared ingest core. Either way: on success the
// connector's last-sync time is stamped and EventPushCompleted goes out (which
// starts a convergence run under the project's on-push policy); on failure the
// error is recorded on the connector row so the status path surfaces it — the
// connector stays bound either way.
func (s *Server) forgeIngest(ctx context.Context, cfg bstore.ConnectorConfig, ev forge.PushEvent, source string) {
	projectID := cfg.Config["project_id"]
	workspaceID := cfg.WorkspaceID
	connectorID := cfg.ID

	// Durable path: enqueue and let bowrain-worker fetch+ingest.
	if s.JobStore != nil && s.JobQueue != nil {
		job := jobs.NewForgeIngestJob(workspaceID, connectorID, projectID, source)
		if err := s.JobStore.CreateJob(ctx, job); err != nil {
			slog.WarnContext(ctx, "forge ingest: durable job create failed; falling back to in-process ingest",
				"connector", connectorID, "project", projectID, "source", source, "error", err)
		} else if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
			_ = s.JobStore.DeleteJob(ctx, job.ID)
			slog.WarnContext(ctx, "forge ingest: durable enqueue failed; falling back to in-process ingest",
				"connector", connectorID, "project", projectID, "source", source, "error", err)
		} else {
			slog.InfoContext(ctx, "forge ingest: enqueued",
				"job_id", job.ID, "connector", connectorID, "project", projectID,
				"source", source, "repo", ev.RepoPath, "branch", ev.Branch)
			return
		}
	}

	// In-process fallback: same shared core, fire-and-forget as before.
	deps := jobs.ForgeIngestDeps{Fetcher: s.Services.Connector, EventBus: s.EventBus}
	if s.ConnectorConfigStore != nil { // typed-nil guard: only set a live recorder
		deps.Recorder = s.ConnectorConfigStore
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 10*time.Minute)
		defer cancel()
		_ = jobs.ForgeIngest(ctx, deps, jobs.ForgeIngestParams{
			WorkspaceID: workspaceID,
			ConnectorID: connectorID,
			ProjectID:   projectID,
			Repo:        ev.RepoPath,
			Branch:      ev.Branch,
			Source:      source,
		})
	}()
}

// HandleGitHubAppWebhook receives push webhooks for every repository the
// server's GitHub App is installed on: POST /api/webhooks/github-app.
//
// One endpoint serves the whole app — GitHub routes all installation events
// here, signed with the app's webhook secret. A push to a repository is
// matched to the forge connector tracking it (by repository path); everything
// downstream is the same flow as the per-connector webhook. Installation
// lifecycle events and pings are acknowledged without action.
func (s *Server) HandleGitHubAppWebhook(c echo.Context) error {
	if s.GitHubApp == nil || s.ConnectorConfigStore == nil || s.Services == nil || s.Services.Connector == nil || s.EventBus == nil {
		return c.NoContent(http.StatusNotFound)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxForgeWebhookBody))
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	if !forge.VerifyGitHubSignature(s.GitHubApp.WebhookSecret(), body, c.Request().Header.Get("X-Hub-Signature-256")) {
		return c.NoContent(http.StatusUnauthorized)
	}

	if c.Request().Header.Get("X-GitHub-Event") != "push" {
		// installation created/removed, ping, … — acknowledged; connectors
		// bind repositories explicitly, so there is nothing to mirror here.
		return c.NoContent(http.StatusAccepted)
	}
	ev, ok := forge.ParsePushEvent(forge.KindGitHub, body)
	if !ok {
		// Silent-202 branches are logged (production drill follow-up): an
		// operator tracing a "GitHub says delivered, nothing happened" report
		// must be able to see which gate dropped the delivery.
		slog.InfoContext(c.Request().Context(), "github app webhook: acknowledged without action",
			"reason", "payload is not a branch push",
			"delivery", c.Request().Header.Get("X-GitHub-Delivery"))
		return c.NoContent(http.StatusAccepted)
	}

	cfg, ok := s.forgeConfigByRepo(c.Request().Context(), ev.RepoPath)
	if !ok {
		// The app is installed on repositories that aren't connected projects;
		// their pushes are simply not ours.
		slog.InfoContext(c.Request().Context(), "github app webhook: acknowledged without action",
			"reason", "no forge connector tracks this repository",
			"repo", ev.RepoPath, "branch", ev.Branch,
			"delivery", c.Request().Header.Get("X-GitHub-Delivery"))
		return c.NoContent(http.StatusAccepted)
	}
	baseBranch := cfg.Config["branch"]
	if baseBranch == "" {
		baseBranch = "main"
	}
	if ev.Branch != baseBranch {
		slog.InfoContext(c.Request().Context(), "github app webhook: acknowledged without action",
			"reason", "pushed branch is not the tracked branch",
			"repo", ev.RepoPath, "branch", ev.Branch, "tracked_branch", baseBranch,
			"connector", cfg.ID,
			"delivery", c.Request().Header.Get("X-GitHub-Delivery"))
		return c.NoContent(http.StatusAccepted)
	}

	s.forgeIngest(c.Request().Context(), cfg, ev, "forge-webhook")
	return c.NoContent(http.StatusAccepted)
}

// forgeConfigByRepo finds the forge connector tracking a GitHub repository
// path (app-level webhooks carry the repository, not a connector id).
func (s *Server) forgeConfigByRepo(ctx context.Context, repoPath string) (bstore.ConnectorConfig, bool) {
	configs, err := s.ConnectorConfigStore.ListAll(ctx)
	if err != nil {
		slog.Warn("github app webhook: config lookup failed", "error", err)
		return bstore.ConnectorConfig{}, false
	}
	for _, cfg := range configs {
		if cfg.Type != "forge" {
			continue
		}
		repo, err := forge.ParseRepo(cfg.Config["repo"])
		if err != nil {
			continue
		}
		if strings.EqualFold(repo.Path, repoPath) {
			return cfg, true
		}
	}
	return bstore.ConnectorConfig{}, false
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

// forgeDeliveryGroup is the durable consumer-group name for run-completion →
// forge delivery. Stable by design (see convergeOnPushGroup): the group's
// resume position on the Redis bus is keyed by this name.
const forgeDeliveryGroup = "forge-delivery"

// subscribeForgeDelivery wires the delivery tier: when a convergence run ends
// converged or parked, every forge connector bound to the project publishes
// the produced translations as a pull/merge request. Failed and canceled runs
// deliver nothing.
//
// Durability: delivery is state-advancing — a run-completed event missed
// during a deploy rollover means the converged translations silently never
// reach the forge as a PR — so it joins a consumer group (resume position
// survives restarts) rather than tailing with Subscribe. The group also gives
// exactly-one-instance handling, so multiple replicas do not each publish the
// same delivery. Replay-safe: ForgeConnector.Publish recreates the single
// delivery branch from the tracked tip on every call and touches nothing when
// the produced files already match the tracked branch, so redelivering an old
// completion event re-publishes the project's current content at worst.
func (s *Server) subscribeForgeDelivery() {
	if s.EventBus == nil || s.Services == nil || s.Services.Connector == nil || s.ConnectorConfigStore == nil {
		return
	}
	s.EventBus.SubscribeGroup(forgeDeliveryGroup, func(ev platev.Event) {
		if ev.Type != platev.EventConvergenceRunCompleted {
			return // group handlers see every event; only run completions matter here
		}
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
// locale has no deliverable translations are skipped rather than delivered
// empty.
//
// Governed projects (workflow_enabled != "false") ship only APPROVED
// translations (RV-A): a reviewed or signed-off target is a person's decision
// to ship; a draft/translated one is not, so unreviewed drafts are withheld
// until approved. A non-governed project keeps the kapi-drafts-ship semantics —
// any committed target delivers — so this path stays byte-for-byte identical
// for it.
func (s *Server) materializeDelivery(ctx context.Context, proj *platstore.Project) ([]*platconn.ContentItem, error) {
	storeItems, err := s.ContentStore.ListItems(ctx, proj.ID, "main")
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	sourceLang := string(proj.DefaultSourceLanguage)
	governed := workflowReviewEnabled(proj)

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
				// Governed delivery ships only approved (reviewed/signed-off)
				// targets; a governed item/locale with zero approved blocks is
				// skipped as empty, exactly as an untranslated one is.
				if governed && !targetApproved(sb.Block, locale) {
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
	b.WriteString("## kapi up report\n\n")
	fmt.Fprintf(&b, "| | |\n|---|---|\n| Outcome | **%s** |\n| Passes | %s |\n| Project | %s |\n",
		state, ev.Data["passes"], proj.Name)
	b.WriteString("\nProduced by the kapi loop running on Bowrain. ")
	if state == bstore.ConvergenceRunParked {
		b.WriteString("Some work parked for a person — it is waiting in the project's review queue and will arrive in a later delivery once approved.")
	} else {
		b.WriteString("Every gated scope cleared its ship gate.")
	}
	return title, b.String()
}
