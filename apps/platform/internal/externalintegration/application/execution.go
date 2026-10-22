package application

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"sync"
	"time"
)

type Authority func(context.Context, uuid.UUID, uuid.UUID, string) (principal.RequestPrincipal, error)
type Executor struct {
	Core      *Service
	Registry  *ports.Registry
	Authority Authority
	Locker    *transaction.Manager
	scanMu    sync.Mutex
	scanAfter uuid.UUID
}

func NewExecutor(core *Service, registry *ports.Registry, authority Authority) *Executor {
	return &Executor{Core: core, Registry: registry, Authority: authority, Locker: core.Tx}
}
func (e *Executor) WithLocker(locker *transaction.Manager) *Executor { e.Locker = locker; return e }
func (e *Executor) principal(ctx context.Context, ws, actor uuid.UUID, subject string) (principal.RequestPrincipal, error) {
	if e.Authority == nil {
		return principal.RequestPrincipal{}, d.ErrDenied
	}
	return Authorize(ctx, ws, func(ctx context.Context) (principal.RequestPrincipal, error) {
		p, err := e.Authority(ctx, ws, actor, subject)
		if err == nil && (p.ActorID != actor || p.Subject != subject) {
			err = d.ErrDenied
		}
		return p, err
	})
}
func (e *Executor) freshCaller(ws uuid.UUID, caller Caller) Caller {
	return func(ctx context.Context) (principal.RequestPrincipal, error) {
		p, err := Authorize(ctx, ws, caller)
		if err != nil {
			return p, err
		}
		return e.principal(ctx, ws, p.ActorID, p.Subject)
	}
}
func (e *Executor) current(ctx context.Context, tx pgx.Tx, a d.Attempt) (principal.RequestPrincipal, d.Publication, error) {
	q := a.Request
	p, err := e.principal(ctx, q.WorkspaceID, a.ActorID, a.PrincipalRef)
	if err != nil {
		return p, d.Publication{}, err
	}
	adapter, err := e.Registry.Get(q.Provider)
	if err != nil || adapter.Instance != q.Instance || adapter.Revision != q.AdapterRevision {
		return p, d.Publication{}, d.ErrUnsupported
	}
	b, err := e.Core.Repo.Binding(ctx, tx, q.WorkspaceID, q.BindingID, true)
	if err != nil {
		return p, d.Publication{}, err
	}
	if b.Status != "BOUND" || b.Provider != q.Provider || b.ExternalSubjectID != q.ExternalSubjectID {
		return p, d.Publication{}, d.ErrState
	}
	if err = e.Core.Repo.Release(ctx, tx, q.WorkspaceID, q.ReleaseID, true); err != nil {
		return p, d.Publication{}, err
	}
	i, err := e.Core.Repo.Identifier(ctx, tx, q.WorkspaceID, q.IdentifierID, true)
	if err != nil {
		return p, d.Publication{}, err
	}
	if i.Status != "RECORDED" || i.Provider != q.Provider || i.ObjectType != "PRODUCT_RELEASE" || i.ObjectID != q.ReleaseID || i.ExternalID != q.ExternalIdentifier {
		return p, d.Publication{}, d.ErrState
	}
	pub, err := e.Core.Repo.Publication(ctx, tx, q.WorkspaceID, q.PublicationID, true)
	if err != nil {
		return p, pub, err
	}
	if pub.Provider != q.Provider || pub.IdentityBindingID != q.BindingID || pub.ProductReleaseID != q.ReleaseID || pub.Status == d.Draft {
		return p, pub, d.ErrState
	}
	return p, pub, nil
}
func (e *Executor) Prepare(ctx context.Context, ws uuid.UUID, caller Caller, key string, pubID uuid.UUID, retry bool) (d.Receipt, error) {
	if pubID == uuid.Nil {
		return d.Receipt{}, d.ErrInvalid
	}
	command := "EXTERNAL.EXECUTE_PUBLICATION"
	if retry {
		command = "EXTERNAL.RETRY_PUBLICATION"
	}
	return e.Core.command(ctx, ws, e.freshCaller(ws, caller), key, command, pubID, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		pub, err := e.Core.Repo.Publication(ctx, tx, ws, pubID, false)
		if err != nil {
			return d.Receipt{}, err
		}
		adapter, err := e.Registry.Get(pub.Provider)
		if err != nil {
			return d.Receipt{}, err
		}
		b, err := e.Core.Repo.Binding(ctx, tx, ws, pub.IdentityBindingID, true)
		if err != nil {
			return d.Receipt{}, err
		}
		if err = e.Core.Repo.Release(ctx, tx, ws, pub.ProductReleaseID, true); err != nil {
			return d.Receipt{}, err
		}
		i, err := e.Core.Repo.ReleaseIdentifier(ctx, tx, ws, pub.Provider, pub.ProductReleaseID)
		if err != nil {
			return d.Receipt{}, err
		}
		pub, err = e.Core.Repo.Publication(ctx, tx, ws, pubID, true)
		if err != nil {
			return d.Receipt{}, err
		}
		if (pub.Status != d.Submitting && pub.Status != d.Registered) || b.Status != "BOUND" || i.Status != "RECORDED" {
			return d.Receipt{}, d.ErrState
		}
		prior, err := e.Core.Repo.LatestAttempt(ctx, tx, ws, pubID)
		number := int64(1)
		if err == nil {
			if !retry || prior.Resolution != "FAILED" {
				return d.Receipt{}, d.ErrState
			}
			number = prior.Number + 1
		} else if !errors.Is(err, d.ErrNotFound) {
			return d.Receipt{}, err
		} else if retry {
			return d.Receipt{}, d.ErrState
		}
		if _, err = e.principal(ctx, ws, p.ActorID, p.Subject); err != nil {
			return d.Receipt{}, err
		}
		id := uuid.New()
		a := d.Attempt{ID: id, Number: number, PrincipalRef: p.Subject, ActorID: p.ActorID, Request: d.Request{WorkspaceID: ws, PublicationID: pubID, ReleaseID: pub.ProductReleaseID, BindingID: b.ID, IdentifierID: i.ID, Provider: pub.Provider, Instance: adapter.Instance, AdapterRevision: adapter.Revision, ExternalSubjectID: b.ExternalSubjectID, ExternalIdentifier: i.ExternalID, RequestKey: "publication/" + pubID.String() + "/attempt/" + id.String()}}
		rec, _, err := e.Core.activity(ctx, tx, p, pubID, id, "ExternalPublication", "ExternalPublicationExecutionRequested", pub.Revision, map[string]any{"attemptId": id, "requestHash": a.Request.Hash(), "provider": pub.Provider})
		if err != nil {
			return rec, err
		}
		a.EvidenceID = rec.EvidenceID
		return rec, e.Core.Repo.InsertAttempt(ctx, tx, a)
	})
}
func (e *Executor) Reconcile(ctx context.Context, ws uuid.UUID, caller Caller, key string, pubID uuid.UUID) (d.Receipt, error) {
	if pubID == uuid.Nil {
		return d.Receipt{}, d.ErrInvalid
	}
	return e.Core.command(ctx, ws, e.freshCaller(ws, caller), key, "EXTERNAL.RECONCILE_PUBLICATION", pubID, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		pub, err := e.Core.Repo.Publication(ctx, tx, ws, pubID, false)
		if err != nil {
			return d.Receipt{}, err
		}
		if _, err = e.Registry.Get(pub.Provider); err != nil {
			return d.Receipt{}, err
		}
		if _, err = e.Core.Repo.LatestAttempt(ctx, tx, ws, pubID); err != nil {
			return d.Receipt{}, err
		}
		rec, _, err := e.Core.activity(ctx, tx, p, pubID, uuid.New(), "ExternalPublication", "ExternalPublicationExecutionRequested", pub.Revision, map[string]any{"wakeup": "RECONCILE"})
		return rec, err
	})
}
func (e *Executor) RunOnce(ctx context.Context) error {
	providers := []string{}
	for _, adapter := range e.Registry.List() {
		providers = append(providers, adapter.Provider)
	}
	if len(providers) == 0 {
		return nil
	}
	e.scanMu.Lock()
	pubs, err := e.Core.Repo.Pending(ctx, providers, e.scanAfter)
	if err == nil && len(pubs) > 0 {
		e.scanAfter = pubs[len(pubs)-1].ID
	}
	e.scanMu.Unlock()
	if err != nil {
		return err
	}
	var first error
	for _, p := range pubs {
		if _, err = e.Registry.Get(p.Provider); err != nil {
			continue
		}
		_, err = e.Core.Repo.LatestAttempt(ctx, e.Core.Repo.Pool, p.WorkspaceID, p.ID)
		if errors.Is(err, d.ErrNotFound) {
			_, err = e.Prepare(ctx, p.WorkspaceID, func(ctx context.Context) (principal.RequestPrincipal, error) {
				return e.principal(ctx, p.WorkspaceID, p.CreatedBy, p.PrincipalRef)
			}, "pending-intent/"+p.ID.String(), p.ID, false)
			if errors.Is(err, d.ErrState) || errors.Is(err, d.ErrConflict) {
				err = nil
			}
		}
		if err == nil {
			err = e.RunPublication(ctx, p.WorkspaceID, p.ID)
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}
func (e *Executor) RunPublication(ctx context.Context, ws, pubID uuid.UUID) error {
	if ws == uuid.Nil || pubID == uuid.Nil {
		return d.ErrInvalid
	}
	err := e.Locker.WithAdvisoryLock(ctx, "external-publication/"+pubID.String(), func(ctx context.Context) error {
		a, err := e.Core.Repo.LatestAttempt(ctx, e.Core.Repo.Pool, ws, pubID)
		if err != nil {
			return err
		}
		if a.Resolution != "OPEN" {
			return nil
		}
		for step := 0; step < 4; step++ {
			calls, err := e.Core.Repo.RecoveryCalls(ctx, e.Core.Repo.Pool, a.ID)
			if err != nil {
				return err
			}
			kind := "IDENTITY"
			var catalog, failed *d.Call
			identity, identifier := false, false
			for idx := range calls {
				c := &calls[idx]
				if c.Kind == "SUBMIT" || c.Kind == "POLL" {
					catalog = c
				}
				if c.Result.State == "FAILED" {
					failed = c
				}
				if c.Result.State == "VERIFIED" {
					if c.Kind == "IDENTITY" {
						identity = true
					}
					if c.Kind == "IDENTIFIER" {
						identifier = true
					}
				}
			}
			if catalog == nil && failed != nil {
				_, err := e.apply(ctx, a, *failed)
				return err
			}
			if catalog != nil {
				if catalog.ObservationID != nil && catalog.Result.State != "UNKNOWN" {
					done, err := e.apply(ctx, a, *catalog)
					if err != nil || done {
						return err
					}
				}
				kind = "POLL"
			} else if identity {
				kind = "IDENTIFIER"
				if identifier {
					kind = "SUBMIT"
				}
			}
			call, err := e.invoke(ctx, a, kind)
			if err != nil {
				return err
			}
			if call.Result.State == "UNKNOWN" {
				return d.ErrUnavailable
			}
			if kind == "SUBMIT" || kind == "POLL" || call.Result.State == "FAILED" {
				done, err := e.apply(ctx, a, call)
				if err != nil || done {
					return err
				}
				// One catalog invocation per wakeup. REGISTERED work waits for the
				// next queue/scanner wakeup rather than polling in a tight loop.
				if kind == "SUBMIT" || kind == "POLL" {
					return nil
				}
			}
		}
		return nil
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		return nil
	}
	return err
}
func recordedPrincipal(a d.Attempt) principal.RequestPrincipal {
	return principal.RequestPrincipal{WorkspaceID: a.Request.WorkspaceID, ActorID: a.ActorID, Subject: a.PrincipalRef}
}
func durableContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
func (e *Executor) invoke(ctx context.Context, a d.Attempt, kind string) (d.Call, error) {
	c := d.Call{ID: uuid.New(), AttemptID: a.ID, Kind: kind}
	q := a.Request
	err := e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, _, err := e.current(ctx, tx, a)
		if err != nil {
			return err
		}
		rec, _, err := e.Core.activity(ctx, tx, p, c.ID, c.ID, "ExternalProviderCall", "ExternalProviderCallStarted", 0, map[string]any{"publicationId": q.PublicationID, "attemptId": a.ID, "kind": kind})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_provider_call(id,attempt_id,workspace_id,kind,evidence_id) VALUES($1,$2,$3,$4,$5)`, c.ID, a.ID, q.WorkspaceID, kind, rec.EvidenceID)
		return err
	})
	if err != nil {
		return c, err
	}
	adapter, err := e.Registry.Get(q.Provider)
	if err != nil {
		return c, err
	}
	invocation := ports.Invocation{ID: c.ID, Dispatch: func(context.Context) error { return e.markDispatch(ctx, a, c) }}
	var result d.ProviderResult
	var providerErr error
	switch kind {
	case "IDENTITY":
		result, providerErr = adapter.Identity.CheckIdentity(ctx, q, invocation)
	case "IDENTIFIER":
		result, providerErr = adapter.Identifier.CheckIdentifier(ctx, q, invocation)
	case "SUBMIT":
		result, providerErr = adapter.Catalog.Submit(ctx, q, invocation)
	case "POLL":
		result, providerErr = adapter.Catalog.Poll(ctx, q, invocation)
	}
	// Provider diagnostics never cross the adapter boundary into Core facts/errors.
	clean, cancel := durableContext(ctx)
	defer cancel()
	err = e.Core.Repo.Pool.QueryRow(clean, `SELECT EXISTS(SELECT 1 FROM external_provider_dispatch WHERE call_id=$1)`, c.ID).Scan(&c.Dispatched)
	if err != nil {
		return c, err
	}
	code := "OK"
	if providerErr != nil {
		code = "PROVIDER_UNAVAILABLE"
	} else if !result.Valid(q, kind) {
		code = "INVALID_PROVIDER_RESULT"
	} else if !c.Dispatched {
		code = "DISPATCH_UNCONFIRMED"
	}
	if code != "OK" {
		result = d.ProviderResult{Instance: q.Instance, RequestKey: q.RequestKey, RequestHash: q.Hash(), State: "UNKNOWN"}
	}
	observation := uuid.New()
	err = e.Core.Tx.Do(clean, func(ctx context.Context, tx pgx.Tx) error {
		if result.State == "REGISTERED" || result.State == "PUBLISHED" {
			var changed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_provider_observation o JOIN external_provider_call c ON c.id=o.call_id JOIN external_publication_attempt a ON a.id=c.attempt_id WHERE a.publication_id=$1 AND o.state IN ('REGISTERED','PUBLISHED') AND o.remote_id<>$2)`, q.PublicationID, result.RemoteID).Scan(&changed); err != nil {
				return err
			}
			if changed {
				code = "INVALID_PROVIDER_RESULT"
				result = d.ProviderResult{Instance: q.Instance, RequestKey: q.RequestKey, RequestHash: q.Hash(), State: "UNKNOWN"}
			}
		}
		rec, _, err := e.Core.activity(ctx, tx, recordedPrincipal(a), q.PublicationID, observation, "ExternalPublication", "ExternalProviderObservationRecorded", 0, map[string]any{"attemptId": a.ID, "callId": c.ID, "kind": kind, "state": result.State, "errorCode": code})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_provider_observation(id,call_id,workspace_id,state,error_code,instance_ref,request_key,request_hash,external_id,remote_id,evidence_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, observation, c.ID, q.WorkspaceID, result.State, code, result.Instance, result.RequestKey, result.RequestHash, result.ExternalID, result.RemoteID, rec.EvidenceID)
		return err
	})
	c.Result = result
	c.Code = code
	c.ObservationID = &observation
	return c, err
}
func (e *Executor) markDispatch(ctx context.Context, a d.Attempt, c d.Call) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	clean, cancel := durableContext(ctx)
	defer cancel()
	return e.Core.Tx.Do(clean, func(ctx context.Context, tx pgx.Tx) error {
		p, _, err := e.current(ctx, tx, a)
		if err != nil {
			return err
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_provider_dispatch WHERE call_id=$1)`, c.ID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return d.ErrConflict
		}
		rec, _, err := e.Core.activity(ctx, tx, p, c.ID, uuid.New(), "ExternalProviderCall", "ExternalProviderCallDispatched", 0, map[string]any{"publicationId": a.Request.PublicationID, "attemptId": a.ID, "kind": c.Kind})
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO external_provider_dispatch(call_id,evidence_id) VALUES($1,$2)`, c.ID, rec.EvidenceID); err != nil {
			return err
		}
		costID := uuid.New()
		if err = cost.Append(ctx, tx, cost.Event{ID: costID, WorkspaceID: a.Request.WorkspaceID, ActivityID: c.ID, CostType: "EXTERNAL_PROVIDER_INVOCATION", Quantity: 1, Unit: "invocation", PricingMode: "ACTUAL"}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,external_provider_call_id) VALUES($1,$2,$3)`, uuid.New(), costID, c.ID)
		return err
	})
}
func (e *Executor) resolution(ctx context.Context, tx pgx.Tx, a d.Attempt, c d.Call, outcome string) error {
	rec, _, err := e.Core.activity(ctx, tx, recordedPrincipal(a), a.Request.PublicationID, uuid.New(), "ExternalPublication", "ExternalPublicationAttemptResolved", 0, map[string]any{"attemptId": a.ID, "providerObservationId": c.ObservationID, "outcome": outcome})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_publication_attempt_resolution(attempt_id,observation_id,workspace_id,outcome,evidence_id) VALUES($1,$2,$3,$4,$5)`, a.ID, c.ObservationID, a.Request.WorkspaceID, outcome, rec.EvidenceID)
	return err
}
func (e *Executor) apply(ctx context.Context, a d.Attempt, c d.Call) (bool, error) {
	if c.ObservationID == nil || c.Result.State == "UNKNOWN" {
		return false, nil
	}
	done := false
	err := e.Core.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, pub, err := e.current(ctx, tx, a)
		if err != nil {
			if !errors.Is(err, d.ErrDenied) && !errors.Is(err, d.ErrState) && !errors.Is(err, d.ErrUnsupported) && !errors.Is(err, principal.ErrCapabilityDenied) && !errors.Is(err, principal.ErrWorkspaceDenied) && !errors.Is(err, principal.ErrNotConfigured) {
				return err
			}
			// Recording already-incurred work is allowed even after the caller is revoked.
			done = true
			return e.resolution(ctx, tx, a, c, "BLOCKED")
		}
		if c.Result.State == "FAILED" || c.Result.State == "ABSENT" {
			done = true
			return e.resolution(ctx, tx, a, c, "FAILED")
		}
		states, err := d.AdvancePublication(pub.Status, c.Result.State)
		if err != nil {
			return err
		}
		for _, next := range states {
			revision := pub.Revision + 1
			fact := uuid.New()
			eventType := "ExternalPublicationRegistered"
			if next == d.Published {
				eventType = "ExternalPublicationPublished"
			}
			rec, event, err := e.Core.activity(ctx, tx, p, pub.ID, fact, "ExternalPublication", eventType, revision, map[string]any{"providerObservationId": c.ObservationID, "attemptId": a.ID, "fromStatus": pub.Status, "toStatus": next})
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE external_publication SET status=$2,revision=$3 WHERE id=$1`, pub.ID, next, revision); err != nil {
				return err
			}
			if err = e.Core.Repo.Transition(ctx, tx, a.Request.WorkspaceID, pub.ID, d.Transition{ID: fact, Revision: revision, FromStatus: pub.Status, ToStatus: next, EvidenceID: rec.EvidenceID, EventID: event, PrincipalRef: p.Subject, CreatedBy: p.ActorID, CreatedAt: time.Now().UTC(), ProviderObservationID: c.ObservationID}); err != nil {
				return err
			}
			pub.Status = next
			pub.Revision = revision
		}
		if pub.Status == d.Published {
			done = true
			return e.resolution(ctx, tx, a, c, "PUBLISHED")
		}
		return nil
	})
	return done, err
}
