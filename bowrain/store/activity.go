package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// ActivityType classifies activities in the feed.
type ActivityType string

const (
	// Content lifecycle
	ActivityItemPushed      ActivityType = "item.pushed"
	ActivityItemPulled      ActivityType = "item.pulled"
	ActivityBlockTranslated ActivityType = "block.translated"
	ActivityBlockReviewed   ActivityType = "block.reviewed"
	ActivityBlockCommented  ActivityType = "block.commented"

	// Project management
	ActivityProjectCreated    ActivityType = "project.created"
	ActivityProjectUpdated    ActivityType = "project.updated"
	ActivityProjectDeleted    ActivityType = "project.deleted"
	ActivityCollectionCreated ActivityType = "collection.created"
	// ActivityRecipeChanged is a change to what a project declares: its
	// collections and their coordinates, its recipe-owned settings, its
	// target languages. Data carries the fields of event.RecipeChange.
	ActivityRecipeChanged ActivityType = "recipe.changed"
	ActivityLocaleAdded   ActivityType = "locale.added"
	ActivityMemberAdded   ActivityType = "member.added"
	ActivityMemberRemoved ActivityType = "member.removed"

	// Stream operations
	ActivityStreamCreated  ActivityType = "stream.created"
	ActivityStreamDeleted  ActivityType = "stream.deleted"
	ActivityStreamMerged   ActivityType = "stream.merged"
	ActivityStreamLocked   ActivityType = "stream.locked"
	ActivityStreamUnlocked ActivityType = "stream.unlocked"
	ActivityStreamTagged   ActivityType = "stream.tagged"

	// Automation & AI
	ActivityFlowCompleted  ActivityType = "flow.completed"
	ActivityFlowFailed     ActivityType = "flow.failed"
	ActivityJobCompleted   ActivityType = "job.completed"
	ActivityJobFailed      ActivityType = "job.failed"
	ActivityExtractionDone ActivityType = "extraction.completed"

	// Quality
	ActivityGatePassed ActivityType = "gate.passed"
	ActivityGateFailed ActivityType = "gate.failed"
	ActivityVoiceDrift ActivityType = "voice.drift"

	// Review queue
	ActivityReviewAssigned ActivityType = "review.assigned"
	ActivityReviewDecided  ActivityType = "review.decided"
	// ActivityReviewSummary is the roll-up of one actor's review decisions
	// in one language and one hour of one project.
	ActivityReviewSummary ActivityType = "review.summary"

	// Knowledge graph (AD-021). The families are the feed's filter vocabulary:
	// `?types=` matches a prefix, so `concept` selects every concept.* entry,
	// and a surface spanning governed change lists changeset, review and pilot.
	ActivityConceptCreated         ActivityType = "concept.created"
	ActivityConceptUpdated         ActivityType = "concept.updated"
	ActivityConceptDeleted         ActivityType = "concept.deleted"
	ActivityConceptTermStatus      ActivityType = "concept.term.status_changed"
	ActivityConceptRelationAdded   ActivityType = "concept.relation.added"
	ActivityConceptRelationRemoved ActivityType = "concept.relation.removed"

	ActivityChangeSetOpened     ActivityType = "changeset.created"
	ActivityChangeSetAbandoned  ActivityType = "changeset.abandoned"
	ActivityChangeSetSuperseded ActivityType = "changeset.superseded"

	ActivityPilotStarted ActivityType = "pilot.started"
	ActivityPilotStopped ActivityType = "pilot.stopped"

	ActivityObservationAdded ActivityType = "observation.added"
	ActivityCommentAdded     ActivityType = "comment.added"

	// Connectors
	ActivityConnectorSynced ActivityType = "connector.synced"
	ActivityConnectorFailed ActivityType = "connector.failed"

	// Tasks
	ActivityTaskCreated    ActivityType = "task.created"
	ActivityTaskCompleted  ActivityType = "task.completed"
	ActivityTaskReassigned ActivityType = "task.reassigned"

	// Versions
	ActivityVersionCreated ActivityType = "version.created"

	// Progress
	ActivityProgressMilestone ActivityType = "progress.milestone"

	// Context: a change to a project's context ledger, or a decision about a
	// voice rule or profile. Data carries the fields of event.ContextChange.
	ActivityContextChanged ActivityType = "context.changed"
)

// FeedKind is the project feed kind an activity belongs to. An activity with
// no kind is listed by the workspace feed and left out of the project feed.
type FeedKind string

// The project feed's kinds: a closed list.
const (
	FeedKindPush              FeedKind = "push"
	FeedKindConvergenceRun    FeedKind = "convergence_run"
	FeedKindReviewSummary     FeedKind = "review_summary"
	FeedKindRecipeChange      FeedKind = "recipe_change"
	FeedKindContextChange     FeedKind = "context_change"
	FeedKindChangeSet         FeedKind = "change_set"
	FeedKindCoverageMilestone FeedKind = "coverage_milestone"
)

// FeedKinds lists the project feed's kinds.
var FeedKinds = []FeedKind{
	FeedKindPush, FeedKindConvergenceRun, FeedKindReviewSummary, FeedKindRecipeChange,
	FeedKindContextChange, FeedKindChangeSet, FeedKindCoverageMilestone,
}

// The actor kinds a feed entry names, in the vocabulary of
// contextop.ActorKind.
const (
	ActorKindPerson = "person"
	ActorKindAgent  = "agent"
	ActorKindTool   = "tool"
)

// Activity is an immutable record of something that happened. A roll-up row
// (Kind review_summary) is the exception: its counts grow as decisions in
// its hour arrive.
type Activity struct {
	ID string `json:"id"`
	// EventID is the bus event this entry records, when it came from one. It
	// makes a redelivered event file one line rather than a second copy.
	EventID     string            `json:"event_id,omitempty"`
	WorkspaceID string            `json:"workspace_id"`
	ProjectID   string            `json:"project_id,omitempty"`
	Stream      string            `json:"stream,omitempty"`
	ActorID     string            `json:"actor_id"`
	ActorName   string            `json:"actor_name"`
	Type        ActivityType      `json:"type"`
	EntityType  string            `json:"entity_type,omitempty"`
	EntityID    string            `json:"entity_id,omitempty"`
	Summary     string            `json:"summary"`
	Data        map[string]string `json:"data"`
	CreatedAt   time.Time         `json:"created_at"`

	// The project feed's fields. Kind is empty for a row outside it.
	Kind FeedKind `json:"kind,omitempty"`
	// ActorKind is person, agent or tool.
	ActorKind string `json:"actor_kind,omitempty"`
	// Session is the agent session behind the entry, when there is one.
	Session string `json:"session,omitempty"`
	// Locales are the languages the entry touches.
	Locales []string `json:"locales,omitempty"`
	// Coordinates is the point the entry sits at (brand, product, channel,
	// market).
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Counts are the entry's typed counts, keyed by what they count.
	Counts map[string]int `json:"counts,omitempty"`
}

// ActivityQuery defines filters for listing activities.
type ActivityQuery struct {
	// WorkspaceID and WorkspaceKeys together name the workspace. Rows are
	// filed under whichever key the event carried, the slug or the id, so a
	// reader passes both and a row under either is listed.
	WorkspaceID   string
	WorkspaceKeys []string
	ProjectID     string
	Stream        string
	ActorID       string
	Type          string   // prefix match (e.g., "block" matches "block.*")
	Types         []string // if set, matches any of these prefixes (overrides Type)
	Since         time.Time
	Limit         int
	Cursor        string // opaque; a previous page's NextCursor

	// The project feed's filters. FeedOnly lists only rows with a kind;
	// Kinds narrows them to some kinds.
	FeedOnly bool
	Kinds    []FeedKind
	// Locale lists rows that touch this language.
	Locale string
	// Coordinates lists rows at every one of these axis values.
	Coordinates map[string]string
	ActorKind   string
}

// ErrInvalidActivityCursor reports a cursor that no List call returned.
var ErrInvalidActivityCursor = errors.New("activity cursor is not one this store returned")

// encodeActivityCursor names the last row of a page by (created_at, id).
// created_at alone cannot: several rows can share a timestamp, and paging
// strictly before it would skip the ones the page did not reach.
func encodeActivityCursor(a Activity) string {
	raw := a.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + a.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeActivityCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", ErrInvalidActivityCursor
	}
	ts, rowID, ok := strings.Cut(string(raw), "|")
	if !ok || rowID == "" {
		return time.Time{}, "", ErrInvalidActivityCursor
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", ErrInvalidActivityCursor
	}
	return t, rowID, nil
}

// distinctKeys returns the distinct, non-empty keys of a workspace.
func distinctKeys(keys ...string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// inClause renders "column IN ($n, ...)" for keys and appends them to args.
func inClause(column string, keys []string, ph func() string, args *[]any) string {
	marks := make([]string, len(keys))
	for i, k := range keys {
		marks[i] = ph()
		*args = append(*args, k)
	}
	return column + " IN (" + strings.Join(marks, ", ") + ")"
}

// ActivityResult is a paginated result set.
type ActivityResult struct {
	Activities []Activity `json:"activities"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// ActivityStore persists activities.
type ActivityStore struct {
	db *sql.DB
}

// NewActivityStore creates a PostgreSQL-backed activity store.
func NewActivityStore(db *sql.DB) *ActivityStore {
	return &ActivityStore{db: db}
}

// activityInsertColumns are the columns Create and AddToRollup write, in the
// order activityInsertArgs returns their values.
const activityInsertColumns = `id, event_id, workspace_id, project_id, stream, actor_id, actor_name,
	type, entity_type, entity_id, summary, data, created_at,
	kind, actor_kind, session, locales, coordinates, counts`

const activityInsertValues = `$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
	$14, $15, $16, $17, $18, $19`

// prepare fills an activity's defaults before it is written.
func (a *Activity) prepare() {
	if a.ID == "" {
		a.ID = id.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	// The column keeps microseconds. Truncating here makes the value the
	// caller holds the value a later read returns.
	a.CreatedAt = a.CreatedAt.UTC().Truncate(time.Microsecond)
	if a.Data == nil {
		a.Data = map[string]string{}
	}
}

func activityInsertArgs(a *Activity) []any {
	marshal := func(v any, empty string) string {
		raw, err := json.Marshal(v)
		if err != nil || string(raw) == "null" {
			return empty
		}
		return string(raw)
	}
	locales := a.Locales
	if locales == nil {
		locales = []string{}
	}
	return []any{
		a.ID, a.EventID, a.WorkspaceID, a.ProjectID, a.Stream, a.ActorID, a.ActorName,
		string(a.Type), a.EntityType, a.EntityID, a.Summary,
		marshal(a.Data, "{}"), a.CreatedAt,
		string(a.Kind), a.ActorKind, a.Session, locales,
		marshal(a.Coordinates, "{}"), marshal(a.Counts, "{}"),
	}
}

// Create inserts a new activity.
func (s *ActivityStore) Create(ctx context.Context, a *Activity) error {
	a.prepare()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO activities (`+activityInsertColumns+`)
		 VALUES (`+activityInsertValues+`)
		 ON CONFLICT DO NOTHING`,
		activityInsertArgs(a)...)
	return err
}

// AddToRollup files the event eventID into the roll-up row a: the row is
// created with a's counts when it does not exist, and a's counts are added
// to the row's when it does. The row's id names the roll-up's key, so every
// event of one key lands on one row. An event already counted is skipped,
// which makes a redelivered event a no-op.
func (s *ActivityStore) AddToRollup(ctx context.Context, eventID string, a *Activity) error {
	if eventID == "" || a.ID == "" {
		return errors.New("a roll-up needs an event id and a row id")
	}
	a.prepare()
	// The row records no single event: many events share it.
	a.EventID = ""

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO activity_rollup_events (event_id, activity_id) VALUES ($1, $2)
		 ON CONFLICT (event_id) DO NOTHING`, eventID, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	// Each count is added key by key; a key only one side holds keeps its
	// value.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO activities (`+activityInsertColumns+`)
		 VALUES (`+activityInsertValues+`)
		 ON CONFLICT (id) DO UPDATE SET counts = (
			SELECT COALESCE(jsonb_object_agg(k,
				COALESCE((activities.counts->>k)::bigint, 0) + COALESCE((EXCLUDED.counts->>k)::bigint, 0)), '{}'::jsonb)
			FROM jsonb_object_keys(activities.counts || EXCLUDED.counts) AS k
		 )`,
		activityInsertArgs(a)...)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// List returns activities matching the query, newest first.
func (s *ActivityStore) List(ctx context.Context, q ActivityQuery) (*ActivityResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}

	var where []string
	var args []any
	n := 0
	ph := func() string { n++; return fmt.Sprintf("$%d", n) }

	if keys := distinctKeys(append([]string{q.WorkspaceID}, q.WorkspaceKeys...)...); len(keys) > 0 {
		where = append(where, inClause("workspace_id", keys, ph, &args))
	}
	if q.ProjectID != "" {
		where = append(where, "project_id = "+ph())
		args = append(args, q.ProjectID)
	}
	if q.Stream != "" {
		where = append(where, "stream = "+ph())
		args = append(args, q.Stream)
	}
	if q.ActorID != "" {
		where = append(where, "actor_id = "+ph())
		args = append(args, q.ActorID)
	}
	// A single prefix cannot express a set that spans families ("review.*" plus
	// "task.*"), so Types ORs one LIKE per prefix.
	if len(q.Types) > 0 {
		likes := make([]string, 0, len(q.Types))
		for _, t := range q.Types {
			if t == "" {
				continue
			}
			likes = append(likes, "type LIKE "+ph())
			args = append(args, t+"%")
		}
		if len(likes) > 0 {
			where = append(where, "("+strings.Join(likes, " OR ")+")")
		}
	} else if q.Type != "" {
		where = append(where, "type LIKE "+ph())
		args = append(args, q.Type+"%")
	}
	if !q.Since.IsZero() {
		where = append(where, "created_at > "+ph())
		args = append(args, q.Since.UTC())
	}
	if q.FeedOnly || len(q.Kinds) > 0 {
		where = append(where, "kind <> ''")
	}
	if len(q.Kinds) > 0 {
		kinds := make([]string, len(q.Kinds))
		for i, k := range q.Kinds {
			kinds[i] = string(k)
		}
		where = append(where, inClause("kind", kinds, ph, &args))
	}
	if q.Locale != "" {
		where = append(where, ph()+" = ANY(locales)")
		args = append(args, q.Locale)
	}
	if len(q.Coordinates) > 0 {
		raw, err := json.Marshal(q.Coordinates)
		if err != nil {
			return nil, err
		}
		where = append(where, "coordinates @> "+ph()+"::jsonb")
		args = append(args, string(raw))
	}
	if q.ActorKind != "" {
		where = append(where, "actor_kind = "+ph())
		args = append(args, q.ActorKind)
	}
	if q.Cursor != "" {
		at, rowID, err := decodeActivityCursor(q.Cursor)
		if err != nil {
			return nil, err
		}
		where = append(where, fmt.Sprintf("(created_at, id) < (%s, %s)", ph(), ph()))
		args = append(args, at, rowID)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(
		`SELECT id, workspace_id, project_id, stream, actor_id, actor_name,
		 type, entity_type, entity_id, summary, data, created_at,
		 kind, actor_kind, session, array_to_json(locales)::text, coordinates::text, counts::text
		 FROM activities %s ORDER BY created_at DESC, id DESC LIMIT %s`, whereClause, ph())
	args = append(args, q.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &ActivityResult{}
	if len(activities) > q.Limit {
		result.Activities = activities[:q.Limit]
		result.NextCursor = encodeActivityCursor(activities[q.Limit-1])
	} else {
		result.Activities = activities
	}

	return result, nil
}

// DailyCounts returns activity counts per day for a workspace since the given
// time. keys are the workspace's slug and id, as for ActivityQuery.
func (s *ActivityStore) DailyCounts(ctx context.Context, since time.Time, keys ...string) ([]DailyCount, error) {
	keys = distinctKeys(keys...)
	if len(keys) == 0 {
		return nil, nil
	}
	n := 0
	ph := func() string { n++; return fmt.Sprintf("$%d", n) }
	var args []any
	keyClause := inClause("workspace_id", keys, ph, &args)
	query := fmt.Sprintf(`SELECT DATE(created_at) AS day, COUNT(*) AS cnt FROM activities
		 WHERE %s AND created_at > %s
		 GROUP BY day ORDER BY day`, keyClause, ph())
	args = append(args, since.UTC())

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []DailyCount
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, dc)
	}
	return counts, rows.Err()
}

// DailyCount is a date + count pair returned by DailyCounts.
type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func scanActivity(row scanner) (*Activity, error) {
	var a Activity
	var typ, dataJSON, kind, localesJSON, coordsJSON, countsJSON string

	err := row.Scan(
		&a.ID, &a.WorkspaceID, &a.ProjectID, &a.Stream,
		&a.ActorID, &a.ActorName, &typ, &a.EntityType,
		&a.EntityID, &a.Summary, &dataJSON, &a.CreatedAt,
		&kind, &a.ActorKind, &a.Session, &localesJSON, &coordsJSON, &countsJSON,
	)
	if err != nil {
		return nil, err
	}

	a.Type = ActivityType(typ)
	a.Kind = FeedKind(kind)
	for _, f := range []struct {
		raw string
		dst any
	}{{localesJSON, &a.Locales}, {coordsJSON, &a.Coordinates}, {countsJSON, &a.Counts}} {
		if f.raw == "" {
			continue
		}
		if err := json.Unmarshal([]byte(f.raw), f.dst); err != nil {
			return nil, fmt.Errorf("activity %s: unmarshal feed fields: %w", a.ID, err)
		}
	}
	if len(a.Locales) == 0 {
		a.Locales = nil
	}
	if len(a.Coordinates) == 0 {
		a.Coordinates = nil
	}
	if len(a.Counts) == 0 {
		a.Counts = nil
	}
	if dataJSON != "" {
		if err := json.Unmarshal([]byte(dataJSON), &a.Data); err != nil {
			return nil, fmt.Errorf("activity %s: unmarshal data: %w", a.ID, err)
		}
	}
	if a.Data == nil {
		a.Data = map[string]string{}
	}

	return &a, nil
}

// GetActivitySeenAt returns when the user last viewed the activity feed.
// Returns zero time if never viewed.
func (s *ActivityStore) GetActivitySeenAt(ctx context.Context, userID, workspaceID string) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT last_seen_at FROM activity_state WHERE user_id = $1 AND workspace_id = $2`,
		userID, workspaceID).Scan(&t)
	if err != nil {
		return time.Time{}, nil // never seen
	}
	return t.UTC(), nil
}

// SetActivitySeenAt records when the user last viewed the activity feed.
func (s *ActivityStore) SetActivitySeenAt(ctx context.Context, userID, workspaceID string, seenAt time.Time) error {
	tsVal := seenAt.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO activity_state (user_id, workspace_id, last_seen_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, workspace_id) DO UPDATE SET last_seen_at = $3`,
		userID, workspaceID, tsVal)
	return err
}

// CountNewActivities returns the number of activities newer than the user's
// last seen timestamp. The seen marker is keyed on workspaceID. Activities are
// matched on workspaceID and on any other key of the same workspace.
func (s *ActivityStore) CountNewActivities(ctx context.Context, userID, workspaceID string, otherKeys ...string) (int, error) {
	seenAt, _ := s.GetActivitySeenAt(ctx, userID, workspaceID)
	if seenAt.IsZero() {
		return 0, nil // never viewed = no indicator (avoid showing dot on first visit)
	}
	keys := distinctKeys(append([]string{workspaceID}, otherKeys...)...)
	n := 0
	ph := func() string { n++; return fmt.Sprintf("$%d", n) }
	var args []any
	keyClause := inClause("workspace_id", keys, ph, &args)
	query := fmt.Sprintf(`SELECT COUNT(*) FROM activities WHERE %s AND created_at > %s`, keyClause, ph())
	args = append(args, seenAt.UTC())

	var count int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}
