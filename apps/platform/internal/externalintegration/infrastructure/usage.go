package infrastructure

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
)

// Recovery loads an observation independently of read-model pagination. A crash
// after recording PAGE must never cause another physical provider call.
func (r *Repository) UsageRecovery(ctx context.Context, ws, pull uuid.UUID) (d.UsageCall, error) {
	var c d.UsageCall
	var raw []byte
	err := r.Pool.QueryRow(ctx, `SELECT c.id,c.pull_id,EXISTS(SELECT 1 FROM external_usage_dispatch WHERE call_id=c.id),o.state,o.page,c.evidence_id FROM external_usage_call c JOIN external_usage_outcome o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.pull_id=$2 ORDER BY CASE WHEN o.state='PAGE' THEN 0 ELSE 1 END,c.created_at,c.id LIMIT 1`, ws, pull).Scan(&c.ID, &c.PullID, &c.Dispatched, &c.State, &raw, &c.EvidenceID)
	if err == nil && len(raw) > 0 {
		var page d.UsagePage
		err = json.Unmarshal(raw, &page)
		c.Page = &page
	}
	return c, Error(err)
}

const sourceSelect = `SELECT s.id,s.workspace_id,s.binding_id,s.provider,s.instance_ref,s.adapter_revision,s.stream,s.external_subject_id,s.actor_id,s.principal_ref,s.evidence_id,COALESCE(cp.revision,0),COALESCE(cp.cursor,'') FROM external_usage_source s LEFT JOIN LATERAL(SELECT revision,cursor FROM external_usage_checkpoint WHERE source_id=s.id ORDER BY revision DESC LIMIT 1)cp ON true`

func scanSource(row pgx.Row) (d.UsageSource, error) {
	var s d.UsageSource
	err := row.Scan(&s.ID, &s.WorkspaceID, &s.BindingID, &s.Provider, &s.Instance, &s.AdapterRevision, &s.Stream, &s.ExternalSubjectID, &s.ActorID, &s.PrincipalRef, &s.EvidenceID, &s.Revision, &s.Cursor)
	return s, Error(err)
}
func (r *Repository) UsageSource(ctx context.Context, q Querier, ws, id uuid.UUID, lock bool) (d.UsageSource, error) {
	sql := sourceSelect + ` WHERE s.workspace_id=$1 AND s.id=$2`
	if lock {
		sql += ` FOR UPDATE OF s`
	}
	s, err := scanSource(q.QueryRow(ctx, sql, ws, id))
	if err == nil && lock {
		err = q.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM external_usage_checkpoint WHERE source_id=$1 ORDER BY revision DESC LIMIT 1),0),COALESCE((SELECT cursor FROM external_usage_checkpoint WHERE source_id=$1 ORDER BY revision DESC LIMIT 1),'')`, id).Scan(&s.Revision, &s.Cursor)
	}
	return s, err
}
func (r *Repository) UsageSources(ctx context.Context, ws uuid.UUID, l, o int) ([]d.UsageSource, error) {
	rows, err := r.Pool.Query(ctx, sourceSelect+` WHERE s.workspace_id=$1 ORDER BY s.created_at,s.id LIMIT $2 OFFSET $3`, ws, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.UsageSource{}
	for rows.Next() {
		v, err := scanSource(rows)
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const pullSelect = `SELECT p.id,p.source_id,p.workspace_id,p.expected_revision,p.from_cursor,COALESCE(r.outcome,'PENDING'),p.evidence_id FROM external_usage_pull p LEFT JOIN external_usage_resolution r ON r.pull_id=p.id`

func scanPull(row pgx.Row) (d.UsagePull, error) {
	var p d.UsagePull
	err := row.Scan(&p.ID, &p.SourceID, &p.WorkspaceID, &p.Revision, &p.Cursor, &p.Outcome, &p.EvidenceID)
	return p, Error(err)
}
func (r *Repository) UsagePull(ctx context.Context, q Querier, ws, id uuid.UUID) (d.UsagePull, error) {
	return scanPull(q.QueryRow(ctx, pullSelect+` WHERE p.workspace_id=$1 AND p.id=$2`, ws, id))
}
func (r *Repository) UsagePulls(ctx context.Context, ws, source uuid.UUID, l, o int) ([]d.UsagePull, error) {
	rows, err := r.Pool.Query(ctx, pullSelect+` WHERE p.workspace_id=$1 AND p.source_id=$2 ORDER BY p.created_at DESC,p.id LIMIT $3 OFFSET $4`, ws, source, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.UsagePull{}
	for rows.Next() {
		v, err := scanPull(rows)
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) PendingUsage(ctx context.Context, after uuid.UUID) ([]d.UsagePull, error) {
	rows, err := r.Pool.Query(ctx, pullSelect+` WHERE r.pull_id IS NULL ORDER BY CASE WHEN p.id>$1 THEN 0 ELSE 1 END,p.id LIMIT 25`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.UsagePull{}
	for rows.Next() {
		v, err := scanPull(rows)
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UsageCalls(ctx context.Context, q Querier, ws, pull uuid.UUID, l, o int) ([]d.UsageCall, error) {
	rows, err := q.Query(ctx, `SELECT c.id,c.pull_id,EXISTS(SELECT 1 FROM external_usage_dispatch WHERE call_id=c.id),COALESCE(x.state,'STARTED'),x.page,c.evidence_id FROM external_usage_call c LEFT JOIN external_usage_outcome x ON x.call_id=c.id WHERE c.workspace_id=$1 AND c.pull_id=$2 ORDER BY c.created_at,c.id LIMIT $3 OFFSET $4`, ws, pull, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.UsageCall{}
	for rows.Next() {
		var v d.UsageCall
		var page []byte
		if err = rows.Scan(&v.ID, &v.PullID, &v.Dispatched, &v.State, &page, &v.EvidenceID); err != nil {
			return out, err
		}
		if len(page) > 0 {
			var p d.UsagePage
			if err = json.Unmarshal(page, &p); err != nil {
				return out, err
			}
			v.Page = &p
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const usageSelect = `SELECT e.id,e.workspace_id,e.source_id,e.provider,e.instance_ref,e.provider_event_id,e.event_source,e.category,e.occurred_at,e.external_subject_id,e.external_identifier,e.remote_id,e.payload_hash,e.evidence_id,COALESCE(d.status,'QUARANTINED'),COALESCE(d.reason,'MAPPING_NOT_FOUND') FROM external_usage_event e LEFT JOIN LATERAL(SELECT status,reason FROM external_usage_decision WHERE event_id=e.id ORDER BY CASE WHEN status='ACCEPTED' THEN 0 ELSE 1 END LIMIT 1)d ON true`

func scanUsage(row pgx.Row) (d.UsageEvent, error) {
	var e d.UsageEvent
	v := &e.UsageOccurrence
	err := row.Scan(&e.ID, &e.WorkspaceID, &e.SourceID, &e.Provider, &e.Instance, &v.ID, &v.Source, &v.Category, &v.OccurredAt, &v.ExternalSubjectID, &v.ExternalIdentifier, &v.RemoteID, &e.PayloadHash, &e.EvidenceID, &e.Status, &e.Reason)
	return e, Error(err)
}
func (r *Repository) UsageEvent(ctx context.Context, q Querier, ws, id uuid.UUID, lock bool) (d.UsageEvent, error) {
	sql := usageSelect + ` WHERE e.workspace_id=$1 AND e.id=$2`
	if lock {
		sql += ` FOR UPDATE OF e`
	}
	return scanUsage(q.QueryRow(ctx, sql, ws, id))
}
func (r *Repository) UsageIdentity(ctx context.Context, q Querier, s d.UsageSource, v d.UsageOccurrence) (d.UsageEvent, error) {
	return scanUsage(q.QueryRow(ctx, usageSelect+` WHERE e.workspace_id=$1 AND e.provider=$2 AND e.instance_ref=$3 AND e.event_source=$4 AND e.provider_event_id=$5`, s.WorkspaceID, s.Provider, s.Instance, v.Source, v.ID))
}
func (r *Repository) UsageEvents(ctx context.Context, ws uuid.UUID, status string, l, o int) ([]d.UsageEvent, error) {
	rows, err := r.Pool.Query(ctx, usageSelect+` WHERE e.workspace_id=$1 AND ($2='' OR d.status=$2) ORDER BY e.created_at,e.id LIMIT $3 OFFSET $4`, ws, status, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.UsageEvent{}
	for rows.Next() {
		v, err := scanUsage(rows)
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UsageDecisions(ctx context.Context, q Querier, ws, event uuid.UUID) ([]d.UsageDecision, error) {
	rows, err := q.Query(ctx, `SELECT id,event_id,status,reason,binding_id,identifier_id,publication_id,release_id,observation_id,evidence_id FROM external_usage_decision WHERE workspace_id=$1 AND event_id=$2 ORDER BY created_at,id LIMIT 2`, ws, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.UsageDecision{}
	for rows.Next() {
		var v d.UsageDecision
		if err = rows.Scan(&v.ID, &v.EventID, &v.Status, &v.Reason, &v.BindingID, &v.IdentifierID, &v.PublicationID, &v.ReleaseID, &v.ObservationID, &v.EvidenceID); err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) MatchUsage(ctx context.Context, q Querier, s d.UsageSource, v d.UsageOccurrence) (d.UsageDecision, error) {
	decision := d.UsageDecision{Status: "QUARANTINED", Reason: "MAPPING_NOT_FOUND"}
	if !v.KnownCategory() {
		decision.Reason = "UNSUPPORTED_CATEGORY"
		return decision, nil
	}
	if v.ExternalSubjectID != s.ExternalSubjectID {
		decision.Reason = "SUBJECT_MISMATCH"
		return decision, nil
	}
	rows, err := q.Query(ctx, `SELECT DISTINCT a.binding_id,a.identifier_id,a.publication_id,a.product_release_id,o.id FROM external_provider_observation o JOIN external_provider_call c ON c.id=o.call_id JOIN external_publication_attempt a ON a.id=c.attempt_id JOIN external_publication p ON p.id=a.publication_id JOIN external_publication_transition t ON t.publication_id=p.id AND t.provider_observation_id=o.id AND t.to_status='PUBLISHED' WHERE a.workspace_id=$1 AND a.provider=$2 AND a.instance_ref=$3 AND a.adapter_revision=$4 AND a.binding_id=$5 AND a.external_subject_id=$6 AND a.external_identifier=$7 AND o.remote_id=$8 AND o.state='PUBLISHED' AND p.status='PUBLISHED' LIMIT 2`, s.WorkspaceID, s.Provider, s.Instance, s.AdapterRevision, s.BindingID, v.ExternalSubjectID, v.ExternalIdentifier, v.RemoteID)
	if err != nil {
		return decision, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
		if err = rows.Scan(&decision.BindingID, &decision.IdentifierID, &decision.PublicationID, &decision.ReleaseID, &decision.ObservationID); err != nil {
			return decision, err
		}
	}
	if err = rows.Err(); err != nil {
		return decision, err
	}
	if n == 1 {
		decision.Status = "ACCEPTED"
		decision.Reason = "MATCHED"
	} else {
		if n > 1 {
			decision.Reason = "AMBIGUOUS_MAPPING"
		}
		decision.BindingID = nil
		decision.IdentifierID = nil
		decision.PublicationID = nil
		decision.ReleaseID = nil
		decision.ObservationID = nil
	}
	return decision, nil
}
func (r *Repository) UsageConflicts(ctx context.Context, ws uuid.UUID, l, o int) ([]map[string]any, error) {
	rows, err := r.Pool.Query(ctx, `SELECT x.id,x.event_id,x.source_id,x.payload_hash,x.evidence_id,o.page->'events'->x.page_index FROM external_usage_conflict x JOIN external_usage_outcome o ON o.call_id=x.call_id WHERE x.workspace_id=$1 ORDER BY x.created_at,x.id LIMIT $2 OFFSET $3`, ws, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, event, source, ev uuid.UUID
		var hash string
		var raw []byte
		if err = rows.Scan(&id, &event, &source, &hash, &ev, &raw); err != nil {
			return out, err
		}
		var v d.UsageOccurrence
		if err = json.Unmarshal(raw, &v); err != nil {
			return out, err
		}
		out = append(out, map[string]any{"id": id, "eventId": event, "sourceId": source, "payloadHash": hash, "evidenceId": ev, "reason": "EVENT_IDENTITY_CONFLICT", "occurrence": v})
	}
	return out, rows.Err()
}
