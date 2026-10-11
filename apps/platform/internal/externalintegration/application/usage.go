package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"sync"
	"time"
)

type UsageImporter struct {
	External  *Executor
	scanMu    sync.Mutex
	scanAfter uuid.UUID
}

func NewUsageImporter(e *Executor) *UsageImporter { return &UsageImporter{External: e} }
func (u *UsageImporter) AuthorizeQuery(ctx context.Context, ws uuid.UUID, c Caller) error {
	_, err := Authorize(ctx, ws, u.External.freshCaller(ws, c))
	return err
}
func (u *UsageImporter) sourceAdapter(s d.UsageSource) (ports.Adapter, error) {
	a, err := u.External.Registry.Get(s.Provider)
	if err != nil || a.Instance != s.Instance || a.Revision != s.AdapterRevision || !a.Capabilities.UsageEvents || a.Usage == nil {
		return a, d.ErrUnsupported
	}
	return a, nil
}
func (u *UsageImporter) current(ctx context.Context, tx pgx.Tx, ws, id uuid.UUID) (d.UsageSource, error) {
	e := u.External
	s, err := e.Core.Repo.UsageSource(ctx, tx, ws, id, false)
	if err != nil {
		return s, err
	}
	if _, err = e.principal(ctx, ws, s.ActorID, s.PrincipalRef); err != nil {
		return s, err
	}
	b, err := e.Core.Repo.Binding(ctx, tx, ws, s.BindingID, true)
	if err != nil {
		return s, err
	}
	s, err = e.Core.Repo.UsageSource(ctx, tx, ws, id, true)
	if err != nil {
		return s, err
	}
	if b.Status != "BOUND" || b.Provider != s.Provider || b.SubjectID != s.ActorID || b.PrincipalRef != s.PrincipalRef || b.ExternalSubjectID != s.ExternalSubjectID {
		return s, d.ErrState
	}
	if _, err = u.sourceAdapter(s); err != nil {
		return s, err
	}
	_, err = e.principal(ctx, ws, s.ActorID, s.PrincipalRef)
	return s, err
}
func usagePrincipal(s d.UsageSource) principal.RequestPrincipal {
	return principal.RequestPrincipal{WorkspaceID: s.WorkspaceID, ActorID: s.ActorID, Subject: s.PrincipalRef}
}
func (u *UsageImporter) CreateSource(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec d.UsageSourceSpec) (d.Receipt, error) {
	e := u.External
	if spec.BindingID == uuid.Nil || !d.ValidStream(spec.Stream) {
		return d.Receipt{}, d.ErrInvalid
	}
	return e.Core.command(ctx, ws, e.freshCaller(ws, caller), key, "EXTERNAL.CREATE_USAGE_SOURCE", spec, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		b, err := e.Core.Repo.Binding(ctx, tx, ws, spec.BindingID, true)
		if err != nil {
			return d.Receipt{}, err
		}
		if b.Status != "BOUND" || b.SubjectID != p.ActorID || b.PrincipalRef != p.Subject {
			return d.Receipt{}, d.ErrDenied
		}
		a, err := e.Registry.Get(b.Provider)
		if err != nil || !a.Capabilities.UsageEvents || a.Usage == nil {
			return d.Receipt{}, d.ErrUnsupported
		}
		if _, err = e.principal(ctx, ws, p.ActorID, p.Subject); err != nil {
			return d.Receipt{}, err
		}
		id := uuid.New()
		rec, _, err := e.Core.activity(ctx, tx, p, id, id, "ExternalUsageSource", "ExternalUsageSourceCreated", 0, map[string]any{"bindingId": b.ID, "provider": b.Provider})
		if err != nil {
			return rec, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_usage_source(id,workspace_id,binding_id,provider,instance_ref,adapter_revision,stream,external_subject_id,actor_id,principal_ref,evidence_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, ws, b.ID, b.Provider, a.Instance, a.Revision, spec.Stream, b.ExternalSubjectID, p.ActorID, p.Subject, rec.EvidenceID)
		return rec, err
	})
}
func (u *UsageImporter) RequestPull(ctx context.Context, ws uuid.UUID, caller Caller, key string, id uuid.UUID) (d.Receipt, error) {
	e := u.External
	if id == uuid.Nil {
		return d.Receipt{}, d.ErrInvalid
	}
	return e.Core.command(ctx, ws, e.freshCaller(ws, caller), key, "EXTERNAL.REQUEST_USAGE_PULL", id, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		s, err := u.current(ctx, tx, ws, id)
		if err != nil {
			return d.Receipt{}, err
		}
		if s.ActorID != p.ActorID || s.PrincipalRef != p.Subject {
			return d.Receipt{}, d.ErrDenied
		}
		pull := uuid.New()
		rec, _, err := e.Core.activity(ctx, tx, p, id, pull, "ExternalUsageSource", "ExternalUsagePullRequested", s.Revision, map[string]any{"sourceId": id, "pullId": pull})
		if err != nil {
			return rec, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_usage_pull(id,source_id,workspace_id,expected_revision,from_cursor,evidence_id)VALUES($1,$2,$3,$4,$5,$6)`, pull, id, ws, s.Revision, s.Cursor, rec.EvidenceID)
		return rec, err
	})
}
func (u *UsageImporter) resolve(ctx context.Context, tx pgx.Tx, s d.UsageSource, p d.UsagePull, outcome string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_usage_resolution WHERE pull_id=$1)`, p.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	rec, _, err := u.External.Core.activity(ctx, tx, usagePrincipal(s), p.ID, uuid.New(), "ExternalUsagePull", "ExternalUsagePullResolved", p.Revision, map[string]any{"sourceId": s.ID, "outcome": outcome})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_usage_resolution(pull_id,source_id,workspace_id,outcome,evidence_id)VALUES($1,$2,$3,$4,$5)`, p.ID, s.ID, s.WorkspaceID, outcome, rec.EvidenceID)
	return err
}
func usageGateError(err error) bool {
	return errors.Is(err, d.ErrDenied) || errors.Is(err, d.ErrState) || errors.Is(err, d.ErrUnsupported) || errors.Is(err, principal.ErrCapabilityDenied) || errors.Is(err, principal.ErrWorkspaceDenied) || errors.Is(err, principal.ErrNotConfigured)
}
func (u *UsageImporter) RunOnce(ctx context.Context) error {
	providers := []string{}
	for _, a := range u.External.Registry.List() {
		// Configured providers may have lost the capability since source creation.
		// RunPull resolves that authority change without dispatching another read.
		providers = append(providers, a.Provider)
	}
	if len(providers) == 0 {
		return nil
	}
	u.scanMu.Lock()
	pulls, err := u.External.Core.Repo.PendingUsage(ctx, providers, u.scanAfter)
	if err == nil && len(pulls) > 0 {
		u.scanAfter = pulls[len(pulls)-1].ID
	}
	u.scanMu.Unlock()
	if err != nil {
		return err
	}
	var first error
	for _, p := range pulls {
		if err = u.RunPull(ctx, p.WorkspaceID, p.ID); err != nil && first == nil {
			first = err
		}
	}
	return first
}
func (u *UsageImporter) RunPull(ctx context.Context, ws, id uuid.UUID) error {
	e := u.External
	p, err := e.Core.Repo.UsagePull(ctx, e.Core.Repo.Pool, ws, id)
	if err != nil {
		return err
	}
	if p.Outcome != "PENDING" {
		return nil
	}
	err = e.Locker.WithAdvisoryLock(ctx, "external-usage/"+p.SourceID.String(), func(ctx context.Context) error {
		var s d.UsageSource
		stopped := false
		err := e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var gate error
			s, gate = u.current(ctx, tx, ws, p.SourceID)
			if gate != nil && !usageGateError(gate) {
				return gate
			}
			fresh, err := e.Core.Repo.UsagePull(ctx, tx, ws, id)
			if err != nil {
				return err
			}
			if fresh.Outcome != "PENDING" {
				stopped = true
				return nil
			}
			if gate != nil {
				stopped = true
				return u.resolve(ctx, tx, s, p, "BLOCKED")
			}
			if s.Revision != p.Revision || s.Cursor != p.Cursor {
				stopped = true
				return u.resolve(ctx, tx, s, p, "STALE")
			}
			return nil
		})
		if err != nil || stopped {
			return err
		}
		q := d.UsageReadRequest{Source: s, Revision: p.Revision, Cursor: p.Cursor, Limit: d.UsagePageLimit}
		saved, err := e.Core.Repo.UsageRecovery(ctx, ws, id)
		if err == nil {
			if saved.Page == nil {
				return e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { return u.resolve(ctx, tx, s, p, "FAILED") })
			}
			return u.apply(ctx, s, p, saved)
		}
		if !errors.Is(err, d.ErrNotFound) {
			return err
		}
		call := d.UsageCall{ID: uuid.New(), PullID: id}
		err = e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			current, err := u.current(ctx, tx, ws, s.ID)
			if err != nil {
				return err
			}
			if current.Revision != p.Revision {
				return d.ErrState
			}
			rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), call.ID, call.ID, "ExternalUsageCall", "ExternalUsageCallStarted", 0, map[string]any{"sourceId": s.ID, "pullId": id})
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO external_usage_call(id,pull_id,source_id,workspace_id,request_hash,evidence_id)VALUES($1,$2,$3,$4,$5,$6)`, call.ID, id, s.ID, ws, q.Hash(), rec.EvidenceID)
			return err
		})
		if err != nil {
			return err
		}
		a, err := u.sourceAdapter(s)
		if err != nil {
			return err
		}
		readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
		page, providerErr := a.Usage.Read(readCtx, q, ports.Invocation{ID: call.ID, Dispatch: func(context.Context) error { return u.dispatch(readCtx, s, p, call) }})
		readCancel()
		clean, cancel := durableContext(ctx)
		defer cancel()
		if err = e.Core.Repo.Pool.QueryRow(clean, `SELECT EXISTS(SELECT 1 FROM external_usage_dispatch WHERE call_id=$1)`, call.ID).Scan(&call.Dispatched); err != nil {
			return err
		}
		call.State = "PAGE"
		if providerErr != nil {
			call.State = "READ_FAILED"
		}
		if !call.Dispatched {
			call.State = "DISPATCH_REJECTED"
		}
		var raw []byte
		if call.State == "PAGE" {
			page, err = page.Normalize(q)
			if err != nil {
				call.State = "INVALID_PAGE"
			} else {
				raw, err = json.Marshal(page)
				if err != nil || len(raw) > 128*1024 {
					call.State = "INVALID_PAGE"
					raw = nil
				} else {
					call.Page = &page
				}
			}
		}
		err = e.Core.Tx.Do(clean, func(ctx context.Context, tx pgx.Tx) error {
			rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), call.ID, uuid.New(), "ExternalUsageCall", "ExternalUsageOutcomeRecorded", 0, map[string]any{"sourceId": s.ID, "pullId": id, "state": call.State})
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO external_usage_outcome(call_id,source_id,workspace_id,state,page,evidence_id)VALUES($1,$2,$3,$4,$5,$6)`, call.ID, s.ID, ws, call.State, raw, rec.EvidenceID)
			return err
		})
		if err != nil {
			return err
		}
		if call.Page == nil {
			return e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { return u.resolve(ctx, tx, s, p, "FAILED") })
		}
		return u.apply(ctx, s, p, call)
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		return nil
	}
	return infra.Error(err)
}
func (u *UsageImporter) dispatch(ctx context.Context, s d.UsageSource, p d.UsagePull, c d.UsageCall) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	clean, cancel := durableContext(ctx)
	defer cancel()
	e := u.External
	return e.Core.Tx.Do(clean, func(ctx context.Context, tx pgx.Tx) error {
		current, err := u.current(ctx, tx, s.WorkspaceID, s.ID)
		if err != nil {
			return err
		}
		if current.Revision != p.Revision {
			return d.ErrState
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_usage_dispatch WHERE call_id=$1)`, c.ID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return d.ErrConflict
		}
		rec, _, err := e.Core.activity(ctx, tx, usagePrincipal(s), c.ID, uuid.New(), "ExternalUsageCall", "ExternalUsageCallDispatched", 0, map[string]any{"sourceId": s.ID, "pullId": p.ID})
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO external_usage_dispatch(call_id,source_id,workspace_id,evidence_id)VALUES($1,$2,$3,$4)`, c.ID, s.ID, s.WorkspaceID, rec.EvidenceID); err != nil {
			return err
		}
		costID := uuid.New()
		if err = cost.Append(ctx, tx, cost.Event{ID: costID, WorkspaceID: s.WorkspaceID, ActivityID: c.ID, CostType: "EXTERNAL_USAGE_INVOCATION", Quantity: 1, Unit: "invocation", PricingMode: "ACTUAL"}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,external_usage_call_id)VALUES($1,$2,$3)`, uuid.New(), costID, c.ID)
		return err
	})
}
