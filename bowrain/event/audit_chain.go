package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// ChainVerification reports the integrity of a hash chain.
type ChainVerification struct {
	ChainKey  string `json:"chain_key"`
	Rows      int    `json:"rows"`
	Valid     bool   `json:"valid"`
	BrokenAt  int64  `json:"broken_at,omitempty"` // audit_log.id of the first tampered/broken row
	BrokenMsg string `json:"broken_msg,omitempty"`
}

// reMarshal normalizes a stored JSON object back to the canonical Go form used
// when the row was hashed. Empty/NULL JSON becomes "".
func reMarshal(stored string) string {
	if stored == "" || stored == "{}" {
		if stored == "{}" {
			return "{}"
		}
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(stored), &m); err != nil {
		return stored // fall back to raw (best effort)
	}
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return stored
	}
	return string(b)
}

// VerifyChain walks a single chain in insertion order and recomputes each row's
// hash from its stored columns, confirming the prev_hash links are intact. A
// mismatch indicates the row (or an earlier one) was tampered with.
func (a *AuditLogger) VerifyChain(ctx context.Context, chainKey string) (ChainVerification, error) {
	result := ChainVerification{ChainKey: chainKey, Valid: true}

	rows, err := a.db.QueryContext(ctx,
		`SELECT id, event_type, actor, source, COALESCE(project_id,''), workspace_id,
		        resource_type, resource_id, effect, data,
		        COALESCE(before_state::text,''), COALESCE(after_state::text,''),
		        request_id, ip, user_agent, causation_id, prev_hash, hash, created_at
		 FROM audit_log WHERE chain_key = $1 ORDER BY id ASC`, chainKey)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	// prevRowHash is the hash of the previously seen retained row. The first
	// retained row is treated as an anchor: its stored prev_hash may point at a
	// pruned (retention-deleted) predecessor, so we verify the row against its
	// OWN stored prev_hash rather than requiring a genesis ("") link. Tampering
	// within the retained window still breaks either the row hash or a link.
	prevRowHash := ""
	first := true
	for rows.Next() {
		var (
			id        int64
			ev        platev.Event
			dataJSON  string
			before    string
			after     string
			storedPv  string
			storedHs  string
			createdAt time.Time
			etype     string
		)
		if err := rows.Scan(&id, &etype, &ev.Actor, &ev.Source, &ev.ProjectID, &ev.WorkspaceID,
			&ev.ResourceType, &ev.ResourceID, &ev.Effect, &dataJSON, &before, &after,
			&ev.RequestID, &ev.IP, &ev.UserAgent, &ev.CausationID, &storedPv, &storedHs, &createdAt); err != nil {
			return result, err
		}
		ev.Type = platev.EventType(etype)
		ev.Timestamp = createdAt
		result.Rows++

		if !first && storedPv != prevRowHash {
			result.Valid = false
			result.BrokenAt = id
			result.BrokenMsg = "prev_hash does not match preceding row (insertion, deletion, or reorder)"
			return result, nil
		}

		canonical := canonicalPayload(ev, reMarshal(dataJSON), reMarshal(before), reMarshal(after))
		sum := sha256.Sum256([]byte(storedPv + "\x1e" + canonical))
		want := hex.EncodeToString(sum[:])
		if want != storedHs {
			result.Valid = false
			result.BrokenAt = id
			result.BrokenMsg = "row hash does not match recomputed hash (tampered or schema drift)"
			return result, nil
		}
		prevRowHash = storedHs
		first = false
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, nil
}

// ListChainKeys returns the distinct chains present, useful for a full audit
// integrity sweep.
func (a *AuditLogger) ListChainKeys(ctx context.Context) ([]string, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT DISTINCT chain_key FROM audit_log ORDER BY chain_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// VerifyAllChains verifies every chain and returns any that fail integrity.
func (a *AuditLogger) VerifyAllChains(ctx context.Context) ([]ChainVerification, error) {
	keys, err := a.ListChainKeys(ctx)
	if err != nil {
		return nil, err
	}
	var broken []ChainVerification
	for _, k := range keys {
		v, err := a.VerifyChain(ctx, k)
		if err != nil {
			return nil, fmt.Errorf("verify chain %q: %w", k, err)
		}
		if !v.Valid {
			broken = append(broken, v)
		}
	}
	return broken, nil
}
