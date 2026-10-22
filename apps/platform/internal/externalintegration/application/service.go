package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"time"
)

type Caller func(context.Context) (principal.RequestPrincipal, error)
type Service struct {
	Tx   *transaction.Manager
	Repo *infra.Repository
}

func New(tx *transaction.Manager, repo *infra.Repository) *Service {
	return &Service{Tx: tx, Repo: repo}
}
func Authorize(ctx context.Context, ws uuid.UUID, caller Caller) (principal.RequestPrincipal, error) {
	if caller == nil {
		return principal.RequestPrincipal{}, d.ErrDenied
	}
	p, err := caller(ctx)
	if err != nil {
		return p, err
	}
	if ws == uuid.Nil || p.WorkspaceID != ws || p.ActorID == uuid.Nil || !d.ValidText(p.Subject, 255) || !p.HasCapability(d.Capability) {
		return p, d.ErrDenied
	}
	return p, nil
}

// Commands are explicit entry points; callers cannot select an arbitrary status.
func (s *Service) BindIdentity(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec d.BindingSpec) (d.Receipt, error) {
	if err := spec.Validate(); err != nil {
		return d.Receipt{}, err
	}
	return s.command(ctx, ws, caller, key, "EXTERNAL.BIND_IDENTITY", spec, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		if err := s.Repo.Basis(ctx, tx, ws, spec.BasisEvidenceID); err != nil {
			return d.Receipt{}, err
		}
		id := uuid.New()
		b := d.Binding{ID: id, WorkspaceID: ws, BindingSpec: spec, SubjectType: "PRINCIPAL", SubjectID: p.ActorID, PrincipalRef: p.Subject, CreatedBy: p.ActorID, CreatedAt: time.Now().UTC()}
		rec, _, err := s.activity(ctx, tx, p, id, id, "ExternalIdentityBinding", "ExternalIdentityBound", 0, map[string]any{"provider": spec.Provider, "subjectId": p.ActorID, "basisEvidenceId": spec.BasisEvidenceID, "assertion": "CALLER_ASSERTED"})
		if err != nil {
			return rec, err
		}
		b.EvidenceID = rec.EvidenceID
		return rec, s.Repo.InsertBinding(ctx, tx, b)
	})
}

type InvalidateSpec struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

func (s *Service) InvalidateIdentity(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec InvalidateSpec) (d.Receipt, error) {
	return s.invalidate(ctx, ws, caller, key, spec, true)
}
func (s *Service) InvalidateIdentifier(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec InvalidateSpec) (d.Receipt, error) {
	return s.invalidate(ctx, ws, caller, key, spec, false)
}
func (s *Service) invalidate(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec InvalidateSpec, identity bool) (d.Receipt, error) {
	if spec.ID == uuid.Nil || !d.ValidText(spec.Reason, 512) {
		return d.Receipt{}, d.ErrInvalid
	}
	command, kind, event, aggregate := "EXTERNAL.INVALIDATE_IDENTIFIER", "IDENTIFIER", "ExternalIdentifierInvalidated", "ExternalIdentifier"
	if identity {
		command, kind, event, aggregate = "EXTERNAL.INVALIDATE_IDENTITY", "IDENTITY", "ExternalIdentityInvalidated", "ExternalIdentityBinding"
	}
	return s.command(ctx, ws, caller, key, command, spec, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		if identity {
			b, err := s.Repo.Binding(ctx, tx, ws, spec.ID, true)
			if err != nil {
				return d.Receipt{}, err
			}
			if b.Status != "BOUND" {
				return d.Receipt{}, d.ErrState
			}
		} else {
			i, err := s.Repo.Identifier(ctx, tx, ws, spec.ID, true)
			if err != nil {
				return d.Receipt{}, err
			}
			if i.Status != "RECORDED" {
				return d.Receipt{}, d.ErrState
			}
		}
		fact := uuid.New()
		rec, _, err := s.activity(ctx, tx, p, spec.ID, fact, aggregate, event, 0, map[string]any{"reason": spec.Reason})
		if err != nil {
			return rec, err
		}
		return rec, s.Repo.Invalidate(ctx, tx, kind, ws, spec.ID, fact, rec.EvidenceID, spec.Reason, p.Subject, p.ActorID, time.Now().UTC())
	})
}
func (s *Service) RecordIdentifier(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec d.IdentifierSpec) (d.Receipt, error) {
	if err := spec.Validate(); err != nil {
		return d.Receipt{}, err
	}
	return s.command(ctx, ws, caller, key, "EXTERNAL.RECORD_IDENTIFIER", spec, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		if err := s.Repo.Basis(ctx, tx, ws, spec.BasisEvidenceID); err != nil {
			return d.Receipt{}, err
		}
		if err := s.Repo.Object(ctx, tx, ws, spec); err != nil {
			return d.Receipt{}, err
		}
		id := uuid.New()
		i := d.Identifier{ID: id, WorkspaceID: ws, IdentifierSpec: spec, PrincipalRef: p.Subject, CreatedBy: p.ActorID, CreatedAt: time.Now().UTC()}
		rec, _, err := s.activity(ctx, tx, p, id, id, "ExternalIdentifier", "ExternalIdentifierRecorded", 0, map[string]any{"provider": spec.Provider, "objectType": spec.ObjectType, "objectId": spec.ObjectID, "basisEvidenceId": spec.BasisEvidenceID, "assertion": "CALLER_ASSERTED"})
		if err != nil {
			return rec, err
		}
		i.EvidenceID = rec.EvidenceID
		return rec, s.Repo.InsertIdentifier(ctx, tx, i)
	})
}
func (s *Service) CreatePublication(ctx context.Context, ws uuid.UUID, caller Caller, key string, spec d.PublicationSpec) (d.Receipt, error) {
	if err := spec.Validate(); err != nil {
		return d.Receipt{}, err
	}
	return s.command(ctx, ws, caller, key, "EXTERNAL.CREATE_PUBLICATION", spec, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		if err := s.currentBinding(ctx, tx, ws, spec); err != nil {
			return d.Receipt{}, err
		}
		if err := s.Repo.Release(ctx, tx, ws, spec.ProductReleaseID, true); err != nil {
			return d.Receipt{}, err
		}
		id, fact := uuid.New(), uuid.New()
		at := time.Now().UTC()
		pub := d.Publication{ID: id, WorkspaceID: ws, PublicationSpec: spec, Status: d.Draft, PrincipalRef: p.Subject, CreatedBy: p.ActorID, CreatedAt: at}
		rec, event, err := s.activity(ctx, tx, p, id, fact, "ExternalPublication", "ExternalPublicationCreated", 0, map[string]any{"provider": spec.Provider, "productReleaseId": spec.ProductReleaseID, "identityBindingId": spec.IdentityBindingID})
		if err != nil {
			return rec, err
		}
		if err := s.Repo.InsertPublication(ctx, tx, pub); err != nil {
			return rec, err
		}
		return rec, s.Repo.Transition(ctx, tx, ws, id, d.Transition{ID: fact, ToStatus: d.Draft, EvidenceID: rec.EvidenceID, EventID: event, PrincipalRef: p.Subject, CreatedBy: p.ActorID, CreatedAt: at})
	})
}
func (s *Service) RequestPublication(ctx context.Context, ws uuid.UUID, caller Caller, key string, id uuid.UUID) (d.Receipt, error) {
	if id == uuid.Nil {
		return d.Receipt{}, d.ErrInvalid
	}
	return s.command(ctx, ws, caller, key, "EXTERNAL.REQUEST_PUBLICATION", id, func(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal) (d.Receipt, error) {
		// Immutable reference read only, then fixed binding -> release -> publication locks.
		pub, err := s.Repo.Publication(ctx, tx, ws, id, false)
		if err != nil {
			return d.Receipt{}, err
		}
		if err := s.currentBinding(ctx, tx, ws, pub.PublicationSpec); err != nil {
			return d.Receipt{}, err
		}
		if err := s.Repo.Release(ctx, tx, ws, pub.ProductReleaseID, true); err != nil {
			return d.Receipt{}, err
		}
		pub, err = s.Repo.Publication(ctx, tx, ws, id, true)
		if err != nil {
			return d.Receipt{}, err
		}
		next, err := d.RequestPublication(pub.Status)
		if err != nil {
			return d.Receipt{}, err
		}
		fact := uuid.New()
		rec, event, err := s.activity(ctx, tx, p, id, fact, "ExternalPublication", "ExternalPublicationRequested", pub.Revision+1, map[string]any{"provider": pub.Provider, "productReleaseId": pub.ProductReleaseID, "identityBindingId": pub.IdentityBindingID, "fromStatus": pub.Status, "toStatus": next, "dispatch": "NOT_STARTED"})
		if err != nil {
			return rec, err
		}
		if err := s.Repo.Submit(ctx, tx, id); err != nil {
			return rec, err
		}
		return rec, s.Repo.Transition(ctx, tx, ws, id, d.Transition{ID: fact, Revision: 1, FromStatus: d.Draft, ToStatus: next, EvidenceID: rec.EvidenceID, EventID: event, PrincipalRef: p.Subject, CreatedBy: p.ActorID, CreatedAt: time.Now().UTC()})
	})
}
func (s *Service) currentBinding(ctx context.Context, tx pgx.Tx, ws uuid.UUID, spec d.PublicationSpec) error {
	b, err := s.Repo.Binding(ctx, tx, ws, spec.IdentityBindingID, true)
	if err != nil {
		return err
	}
	if b.Provider != spec.Provider || b.Status != "BOUND" {
		return d.ErrState
	}
	return nil
}
func (s *Service) command(ctx context.Context, ws uuid.UUID, caller Caller, key, command string, spec any, fn func(context.Context, pgx.Tx, principal.RequestPrincipal) (d.Receipt, error)) (d.Receipt, error) {
	if !d.ValidText(key, 255) {
		return d.Receipt{}, d.ErrInvalid
	}
	var rec d.Receipt
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := Authorize(ctx, ws, caller)
		if err != nil {
			return err
		}
		fp, err := d.Fingerprint(struct {
			WorkspaceID  uuid.UUID
			ActorID      uuid.UUID
			PrincipalRef string
			Spec         any
		}{ws, p.ActorID, p.Subject, spec})
		if err != nil {
			return err
		}
		prior, replay, err := s.Repo.Replay(ctx, tx, ws, command, key, fp)
		if err != nil {
			return err
		}
		if replay {
			rec = prior
			return nil
		}
		rec, err = fn(ctx, tx, p)
		if err != nil {
			return err
		}
		return s.Repo.SaveReceipt(ctx, tx, ws, command, key, fp, rec)
	})
	return rec, infra.Error(err)
}
func (s *Service) activity(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal, object, fact uuid.UUID, aggregate, eventType string, revision int64, extra map[string]any) (d.Receipt, uuid.UUID, error) {
	payload := map[string]any{"workspaceId": p.WorkspaceID, "objectId": object, "factId": fact, "principalRef": p.Subject, "actorId": p.ActorID, "revision": revision}
	for k, v := range extra {
		payload[k] = v
	}
	rec, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: p.WorkspaceID, EvidenceType: "EXTERNAL_INTEGRATION", Title: eventType, SourceType: aggregate, SourceID: &fact, CreatedBy: &p.ActorID, Metadata: payload}, evidence.Relation{ObjectType: aggregate, ObjectID: object, RelationType: "LIFECYCLE_FACT"})
	result := d.Receipt{ObjectID: object, FactID: fact, EvidenceID: rec.ID}
	if err != nil {
		return result, uuid.Nil, err
	}
	if release, ok := extra["productReleaseId"].(uuid.UUID); ok {
		_, err := tx.Exec(ctx, `INSERT INTO evidence_relation(evidence_id,object_type,object_id,relation_type) VALUES($1,'ProductRelease',$2,'EXTERNAL_PUBLICATION')`, rec.ID, release)
		if err != nil {
			return result, uuid.Nil, err
		}
	}
	if err := audit.Append(ctx, tx, audit.Event{WorkspaceID: &p.WorkspaceID, ActorType: "USER", ActorID: &p.ActorID, Action: eventType, ObjectType: aggregate, ObjectID: object, AfterState: payload, Metadata: map[string]any{"evidenceId": rec.ID, "principalRef": p.Subject}}); err != nil {
		return result, uuid.Nil, err
	}
	event, err := outbox.NewEvent(aggregate, object, eventType, payload)
	if err != nil {
		return result, uuid.Nil, err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return result, uuid.Nil, err
	}
	return result, event.ID, nil
}
