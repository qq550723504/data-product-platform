package application

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

func (u *UsageImporter) decision(ctx context.Context, tx pgx.Tx, s d.UsageSource, event uuid.UUID, v d.UsageOccurrence) (d.Receipt, error) {
	e := u.External
	dec, err := e.Core.Repo.MatchUsage(ctx, tx, s, v)
	if err != nil {
		return d.Receipt{}, err
	}
	dec.ID = uuid.New()
	dec.EventID = event
	eventType := "ExternalUsageQuarantined"
	if dec.Status == "ACCEPTED" {
		eventType = "ExternalUsageAccepted"
	}
	rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), event, dec.ID, "ExternalUsageEvent", eventType, 0, map[string]any{"sourceId": s.ID, "status": dec.Status, "reason": dec.Reason, "releaseId": dec.ReleaseID, "publicationId": dec.PublicationID})
	if err != nil {
		return rec, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_usage_decision(id,event_id,source_id,workspace_id,status,reason,binding_id,identifier_id,publication_id,release_id,observation_id,evidence_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, dec.ID, event, s.ID, s.WorkspaceID, dec.Status, dec.Reason, dec.BindingID, dec.IdentifierID, dec.PublicationID, dec.ReleaseID, dec.ObservationID, rec.EvidenceID)
	if err != nil {
		return rec, err
	}
	if dec.Status == "ACCEPTED" {
		_, err = tx.Exec(ctx, `INSERT INTO evidence_relation(evidence_id,object_type,object_id,relation_type)VALUES($1,'ExternalPublication',$2,'DOWNSTREAM_RUNTIME'),($1,'ProductRelease',$3,'DOWNSTREAM_RUNTIME')`, rec.EvidenceID, dec.PublicationID, dec.ReleaseID)
	}
	return rec, err
}
func (u *UsageImporter) apply(ctx context.Context, s d.UsageSource, p d.UsagePull, c d.UsageCall) error {
	e := u.External
	return e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := u.current(ctx, tx, s.WorkspaceID, s.ID)
		if err != nil {
			if usageGateError(err) {
				return u.resolve(ctx, tx, s, p, "BLOCKED")
			}
			return err
		}
		fresh, err := e.Core.Repo.UsagePull(ctx, tx, s.WorkspaceID, p.ID)
		if err != nil {
			return err
		}
		if fresh.Outcome != "PENDING" {
			return nil
		}
		if current.Revision != p.Revision || current.Cursor != p.Cursor {
			return u.resolve(ctx, tx, s, p, "STALE")
		}
		q := d.UsageReadRequest{Source: current, Revision: p.Revision, Cursor: p.Cursor, Limit: d.UsagePageLimit}
		page, err := c.Page.Normalize(q)
		if err != nil {
			return err
		}
		for index, v := range page.Events {
			old, err := e.Core.Repo.UsageIdentity(ctx, tx, s, v)
			if err == nil {
				if old.PayloadHash == v.Hash() {
					continue
				}
				var exists bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_usage_conflict WHERE event_id=$1 AND payload_hash=$2)`, old.ID, v.Hash()).Scan(&exists); err != nil {
					return err
				}
				if exists {
					continue
				}
				id := uuid.New()
				rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), id, id, "ExternalUsageConflict", "ExternalUsageConflictQuarantined", 0, map[string]any{"sourceId": s.ID, "eventId": old.ID, "payloadHash": v.Hash()})
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO external_usage_conflict(id,event_id,source_id,workspace_id,call_id,page_index,payload_hash,evidence_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, old.ID, s.ID, s.WorkspaceID, c.ID, index, v.Hash(), rec.EvidenceID); err != nil {
					return err
				}
				continue
			}
			if !errors.Is(err, d.ErrNotFound) {
				return err
			}
			id := uuid.New()
			rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), id, id, "ExternalUsageEvent", "ExternalUsageRecorded", 0, map[string]any{"sourceId": s.ID, "provider": s.Provider, "category": v.Category, "payloadHash": v.Hash()})
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO external_usage_event(id,source_id,workspace_id,call_id,page_index,provider,instance_ref,event_source,provider_event_id,category,occurred_at,external_subject_id,external_identifier,remote_id,payload_hash,evidence_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id, s.ID, s.WorkspaceID, c.ID, index, s.Provider, s.Instance, v.Source, v.ID, v.Category, v.OccurredAt, v.ExternalSubjectID, v.ExternalIdentifier, v.RemoteID, v.Hash(), rec.EvidenceID); err != nil {
				return err
			}
			if _, err = u.decision(ctx, tx, s, id, v); err != nil {
				return err
			}
		}
		if _, err = e.principal(ctx, s.WorkspaceID, s.ActorID, s.PrincipalRef); err != nil {
			return err
		}
		rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), s.ID, uuid.New(), "ExternalUsageSource", "ExternalUsageCheckpointCommitted", p.Revision+1, map[string]any{"pullId": p.ID, "callId": c.ID})
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO external_usage_checkpoint(source_id,workspace_id,revision,pull_id,call_id,cursor,evidence_id)VALUES($1,$2,$3,$4,$5,$6,$7)`, s.ID, s.WorkspaceID, p.Revision+1, p.ID, c.ID, page.NextCursor, rec.EvidenceID); err != nil {
			return err
		}
		return u.resolve(ctx, tx, s, p, "IMPORTED")
	})
}
func (u *UsageImporter) Recheck(ctx context.Context, ws uuid.UUID, caller Caller, key string, id uuid.UUID) (d.Receipt, error) {
	e := u.External
	if id == uuid.Nil {
		return d.Receipt{}, d.ErrInvalid
	}
	return e.Core.command(ctx, ws, e.freshCaller(ws, caller), key, "EXTERNAL.RECHECK_USAGE_EVENT", id, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		v, err := e.Core.Repo.UsageEvent(ctx, tx, ws, id, false)
		if err != nil {
			return d.Receipt{}, err
		}
		s, err := u.current(ctx, tx, ws, v.SourceID)
		if err != nil {
			return d.Receipt{}, err
		}
		if p.ActorID != s.ActorID || p.Subject != s.PrincipalRef {
			return d.Receipt{}, d.ErrDenied
		}
		if _, err = e.Core.Repo.UsageEvent(ctx, tx, ws, id, true); err != nil {
			return d.Receipt{}, err
		}
		decisions, err := e.Core.Repo.UsageDecisions(ctx, tx, ws, id)
		if err != nil {
			return d.Receipt{}, err
		}
		for _, dec := range decisions {
			if dec.Status == "ACCEPTED" {
				return d.Receipt{ObjectID: id, FactID: dec.ID, EvidenceID: dec.EvidenceID}, nil
			}
		}
		match, err := e.Core.Repo.MatchUsage(ctx, tx, s, v.UsageOccurrence)
		if err != nil {
			return d.Receipt{}, err
		}
		if match.Status != "ACCEPTED" {
			return d.Receipt{}, d.ErrState
		}
		return u.decision(ctx, tx, s, id, v.UsageOccurrence)
	})
}
