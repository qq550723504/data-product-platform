package application

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// Caller resolves current trusted authority on EVERY boundary, never from a
// stored bearer secret or a body actor ID. HTTP composition reuses Resolver.
type Caller func(context.Context, uuid.UUID) (principal.RequestPrincipal, error)
type Engine interface {
	Version() string
	Check(context.Context, d.Request) error
	Register(context.Context, d.Operation) (string, error)
	Start(context.Context, d.Operation) error
	Status(context.Context, d.Operation) (string, error)
	Manifest(context.Context, d.Operation) (d.Manifest, error)
}
type Store interface {
	Get(context.Context, string) (io.ReadCloser, error)
	PutImmutable(context.Context, string, io.Reader, int64, string) (string, error)
}
type Service struct {
	Tx       *transaction.Manager
	Repo     *infra.Repository
	Datasets *datasetinfra.PostgresRepository
	Engine   Engine
	Store    Store
	Bucket   string
}

func New(tx *transaction.Manager, repo *infra.Repository, ds *datasetinfra.PostgresRepository, engine Engine, store Store, bucket string) *Service {
	return &Service{tx, repo, ds, engine, store, bucket}
}

func (s *Service) check(ctx context.Context, tx pgx.Tx, r d.Request, caller Caller) error {
	if caller == nil || s.Engine == nil || s.Store == nil {
		return d.ErrDenied
	}
	p, err := caller(ctx, r.WorkspaceID)
	if err != nil {
		return d.ErrDenied
	}
	if p.ActorID != r.ActorID || p.Subject != r.PrincipalRef || p.WorkspaceID != r.WorkspaceID || !p.HasCapability("INGEST_RAW") {
		return d.ErrDenied
	}
	if err := r.Validate(); err != nil {
		return err
	}
	// Resource locks fence current Core revocations through the final commit.
	slots := slices.Clone(r.Slots)
	slices.SortFunc(slots, func(a, b d.Slot) int { return strings.Compare(a.ResourceID.String(), b.ResourceID.String()) })
	for _, slot := range slots {
		var allowed bool
		err := tx.QueryRow(ctx, `SELECT workspace_id=$2 AND lifecycle_status NOT IN ('BLOCKED','ARCHIVED') AND deleted_at IS NULL FROM data_resource WHERE id=$1 FOR UPDATE`, slot.ResourceID, r.WorkspaceID).Scan(&allowed)
		if err != nil || !allowed {
			return d.ErrDenied
		}
	}
	slices.SortFunc(slots, func(a, b d.Slot) int { return strings.Compare(a.DatasetID.String(), b.DatasetID.String()) })
	for _, slot := range slots {
		var allowed bool
		err := tx.QueryRow(ctx, `SELECT workspace_id=$2 AND dataset_type='RAW' AND source_resource_id=$3 AND lifecycle_status='ACTIVE' AND deleted_at IS NULL FROM dataset WHERE id=$1 FOR UPDATE`, slot.DatasetID, r.WorkspaceID, slot.ResourceID).Scan(&allowed)
		if err != nil || !allowed {
			return d.ErrDenied
		}
	}
	// Fresh source read privilege/config/schema check; failures stay fail closed.
	if err := s.Engine.Check(ctx, r); err != nil {
		return d.ErrDenied
	}
	return nil
}
func (s *Service) fact(ctx context.Context, tx pgx.Tx, op d.Operation, event string, payload any) error {
	e, err := outbox.NewEvent("INGESTION_OPERATION", op.ID, event, payload)
	if err != nil {
		return err
	}
	if err = outbox.Append(ctx, tx, e); err != nil {
		return err
	}
	if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &op.Request.WorkspaceID, ActorType: "USER", ActorID: &op.Request.ActorID, Action: event, ObjectType: "INGESTION_OPERATION", ObjectID: op.ID, AfterState: payload}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: op.Request.WorkspaceID, EvidenceType: "INGESTION_ACTIVITY", Title: event, SourceType: "INGESTION_OPERATION", SourceID: &op.ID, CreatedBy: &op.Request.ActorID, Metadata: map[string]any{"event": event, "fact": payload}}, evidence.Relation{ObjectType: "INGESTION_OPERATION", ObjectID: op.ID, RelationType: "SUPPORTS"})
	return err
}
func (s *Service) Prepare(ctx context.Context, r d.Request, caller Caller) (d.Operation, error) {
	var op d.Operation
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := r.Validate(); err != nil {
			return err
		}
		var fresh bool
		var err error
		op, fresh, err = s.Repo.Prepare(ctx, tx, r)
		if err != nil {
			return err
		}
		if err := s.check(ctx, tx, r, caller); err != nil {
			return err
		}
		if !fresh {
			return nil
		}
		return s.fact(ctx, tx, op, "IngestionPrepared", map[string]any{"attemptId": op.AttemptID})
	})
	return op, err
}
func (s *Service) invoke(ctx context.Context, id uuid.UUID, kind string, caller Caller) (d.Operation, error) {
	var op d.Operation
	var invocation uuid.UUID
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		op, err = s.Repo.Load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err = s.check(ctx, tx, op.Request, caller); err != nil {
			return err
		}
		if (kind == "REGISTER" && op.State != "PREPARED") || (kind == "START" && op.State != "REGISTERED") || (kind == "STATUS" && (op.RemoteID == "" || (op.State != "STARTING" && op.State != "RUNNING" && op.State != "UNKNOWN"))) {
			return d.ErrState
		}
		if kind == "STATUS" {
			var started bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingestion_invocation WHERE attempt_id=$1 AND kind='START')`, op.AttemptID).Scan(&started); err != nil {
				return err
			}
			if !started {
				return d.ErrState
			}
		}
		invocation, err = s.Repo.Claim(ctx, tx, &op, kind)
		if err != nil {
			return err
		}
		return s.fact(ctx, tx, op, "IngestionInvocationStarted", map[string]any{"invocationId": invocation, "attemptId": op.AttemptID, "kind": kind})
	})
	if err != nil {
		return op, err
	}
	state, remote := "UNKNOWN", op.RemoteID
	var m *d.Manifest
	switch kind {
	case "REGISTER":
		remote, err = s.Engine.Register(ctx, op)
		if err == nil && remote != "" {
			state = "REGISTERED"
		}
	case "START":
		err = s.Engine.Start(ctx, op)
		if err == nil {
			state = "RUNNING"
		}
	case "STATUS":
		state, err = s.Engine.Status(ctx, op)
		if err != nil {
			state = "UNKNOWN"
		}
		if state == "SUCCEEDED" {
			var manifest d.Manifest
			manifest, err = s.Engine.Manifest(ctx, op)
			if err == nil {
				err = manifest.Validate(op, s.Bucket, s.Engine.Version())
			}
			if err != nil {
				state = "UNKNOWN"
			} else {
				m = &manifest
			}
		}
	}
	if state != "REGISTERED" && state != "RUNNING" && state != "SUCCEEDED" && state != "FAILED" {
		state = "UNKNOWN"
	}
	// Use a bounded detached context to retain a lost/cancelled call observation.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30_000_000_000)
	defer cancel()
	finishErr := s.Tx.Do(finishCtx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := s.Repo.Load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.AttemptID != op.AttemptID {
			return d.ErrConflict
		}
		// Concurrent status calls may not regress an already durable completion.
		if current.State == "SUCCEEDED" || current.State == "ACCEPTED" || current.State == "FAILED" {
			_, err := tx.Exec(ctx, `INSERT INTO ingestion_observation(id,invocation_id,state,remote_id) VALUES($1,$2,$3,$4)`, uuid.New(), invocation, state, remote)
			op = current
			if err != nil {
				return err
			}
			return s.fact(ctx, tx, op, "IngestionObserved", map[string]any{"invocationId": invocation, "attemptId": op.AttemptID, "state": state, "late": true})
		}
		if err = s.Repo.Observe(ctx, tx, &current, invocation, state, remote, m); err != nil {
			return err
		}
		op = current
		return s.fact(ctx, tx, op, "IngestionObserved", map[string]any{"invocationId": invocation, "attemptId": op.AttemptID, "state": state})
	})
	if finishErr != nil {
		return op, finishErr
	}
	if state == "UNKNOWN" {
		return op, d.ErrUnknown
	}
	return op, nil
}

// Dispatch issues each physical register/start at most once. A durable claim
// without a reply is never interpreted as permission to repeat the side effect.
func (s *Service) Dispatch(ctx context.Context, id uuid.UUID, caller Caller) (d.Operation, error) {
	op, err := s.Repo.Load(ctx, s.Repo.Pool, id, false)
	if err != nil {
		return op, err
	}
	if op.State == "PREPARED" {
		op, err = s.invoke(ctx, id, "REGISTER", caller)
		if err != nil {
			return op, err
		}
	}
	if op.State == "REGISTERED" {
		return s.invoke(ctx, id, "START", caller)
	}
	return op, d.ErrState
}
func (s *Service) Recover(ctx context.Context, id uuid.UUID, caller Caller) (d.Operation, error) {
	op, err := s.Repo.Load(ctx, s.Repo.Pool, id, false)
	if err != nil {
		return op, err
	}
	if op.RemoteID == "" {
		if err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { return s.check(ctx, tx, op.Request, caller) }); err != nil {
			return op, err
		}
		return op, d.ErrUnknown
	}
	if op.State == "SUCCEEDED" || op.State == "ACCEPTED" {
		err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { return s.check(ctx, tx, op.Request, caller) })
		return op, err
	}
	return s.invoke(ctx, id, "STATUS", caller)
}
func (s *Service) RetryFailed(ctx context.Context, id uuid.UUID, caller Caller) (d.Operation, error) {
	var op d.Operation
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		op, err = s.Repo.Load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if op.State != "FAILED" {
			return d.ErrState
		}
		if err = s.check(ctx, tx, op.Request, caller); err != nil {
			return err
		}
		if err = s.Repo.NewAttempt(ctx, tx, &op); err != nil {
			return err
		}
		return s.fact(ctx, tx, op, "IngestionPrepared", map[string]any{"attemptId": op.AttemptID, "retry": true})
	})
	return op, err
}
func (s *Service) bytes(ctx context.Context, uri string, out d.Output) ([]byte, error) {
	r, err := s.Store.Get(ctx, uri)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, d.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if err = d.ValidateContent(out, data); err != nil {
		return nil, err
	}
	return data, nil
}
func (s *Service) Admit(ctx context.Context, id uuid.UUID, m d.Manifest, caller Caller) (d.Receipt, error) {
	op, err := s.Repo.Load(ctx, s.Repo.Pool, id, false)
	if err != nil {
		return d.Receipt{}, err
	}
	if err = m.Validate(op, s.Bucket, s.Engine.Version()); err != nil {
		return d.Receipt{}, err
	}
	_, hash, err := m.Canonical()
	if err != nil {
		return d.Receipt{}, err
	}
	if op.Manifest == nil {
		return d.Receipt{}, d.ErrUnknown
	}
	_, saved, _ := op.Manifest.Canonical()
	if saved != hash {
		return d.Receipt{}, d.ErrConflict
	}
	if op.State != "SUCCEEDED" && op.State != "ACCEPTED" {
		return d.Receipt{}, d.ErrState
	}
	if err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error { return s.check(ctx, tx, op.Request, caller) }); err != nil {
		return d.Receipt{}, err
	}
	if op.State == "ACCEPTED" {
		return s.Repo.Receipt(ctx, s.Repo.Pool, id)
	}
	// Complete all object writes and verify durable bytes BEFORE allocating any
	// versions. A lost PUT response is reconciled by GET/hash, never overwrite.
	uris := map[string]string{}
	for _, out := range m.Outputs {
		data, err := s.bytes(ctx, out.URI, out)
		if err != nil {
			return d.Receipt{}, err
		}
		key := d.CoreKey(op, out)
		uri := "s3://" + s.Bucket + "/" + key
		_, putErr := s.Store.PutImmutable(ctx, key, bytes.NewReader(data), int64(len(data)), "text/csv")
		if _, err = s.bytes(ctx, uri, out); err != nil {
			if putErr != nil {
				return d.Receipt{}, fmt.Errorf("immutable object unavailable")
			}
			return d.Receipt{}, err
		}
		uris[out.Code] = uri
	}
	var rec d.Receipt
	err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Shared workspace fence first, then operation/resources/datasets, matching
		// Dataset SetReady's existing fence and parent-lock order.
		if _, err := deliveryfence.Lock(ctx, tx, op.Request.WorkspaceID); err != nil {
			return err
		}
		current, err := s.Repo.Load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err = s.check(ctx, tx, current.Request, caller); err != nil {
			return err
		}
		if current.AttemptID != op.AttemptID || current.Manifest == nil {
			return d.ErrConflict
		}
		_, h, _ := current.Manifest.Canonical()
		if h != hash {
			return d.ErrConflict
		}
		if current.State == "ACCEPTED" {
			rec, err = s.Repo.Receipt(ctx, tx, id)
			return err
		}
		if current.State != "SUCCEEDED" {
			return d.ErrState
		}
		rec = d.Receipt{ID: uuid.New(), OperationID: id, AttemptID: op.AttemptID, ManifestHash: hash}
		ev, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: op.Request.WorkspaceID, EvidenceType: "INGESTION_MANIFEST", Title: "Verified RAW batch", SourceType: "INGESTION_OPERATION", SourceID: &id, CreatedBy: &op.Request.ActorID, Metadata: map[string]any{"manifest": m, "manifestHash": hash}}, evidence.Relation{ObjectType: "INGESTION_OPERATION", ObjectID: id, RelationType: "SUPPORTS"})
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO ingestion_receipt(id,operation_id,attempt_id,manifest_hash,evidence_id) VALUES($1,$2,$3,$4,$5)`, rec.ID, id, op.AttemptID, hash, ev.ID); err != nil {
			return err
		}
		outputs := slices.Clone(m.Outputs)
		slices.SortFunc(outputs, func(a, b d.Output) int { return strings.Compare(a.DatasetID.String(), b.DatasetID.String()) })
		for _, out := range outputs {
			// Reuse transaction-capable existing allocation/publication primitives.
			v, _, err := s.Datasets.AllocateVersion(ctx, tx, out.DatasetID, &op.Request.ActorID, nil, nil)
			if err != nil {
				return err
			}
			if err = v.MarkReady("OBJECT_STORAGE", uris[out.Code], "text/csv", "SHA256", out.SHA256, out.Rows, out.Bytes); err != nil {
				return err
			}
			prev, err := s.Datasets.SetReady(ctx, tx, v)
			if err != nil {
				return err
			}
			if prev != nil {
				e, err := outbox.NewEvent("DATASET_VERSION", prev.ID, "DatasetVersionSuperseded", map[string]any{"replacementVersionId": v.ID})
				if err != nil {
					return err
				}
				if err = outbox.Append(ctx, tx, e); err != nil {
					return err
				}
				if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &op.Request.WorkspaceID, ActorType: "USER", ActorID: &op.Request.ActorID, Action: "DATASET_VERSION_SUPERSEDED", ObjectType: "DATASET_VERSION", ObjectID: prev.ID, AfterState: map[string]any{"replacementVersionId": v.ID}}); err != nil {
					return err
				}
			}
			b := d.Binding{Code: out.Code, DatasetID: out.DatasetID, VersionID: v.ID, URI: uris[out.Code], SHA256: out.SHA256}
			rec.Bindings = append(rec.Bindings, b)
			if _, err = tx.Exec(ctx, `INSERT INTO ingestion_receipt_slot(receipt_id,code,dataset_id,version_id,storage_uri,content_hash) VALUES($1,$2,$3,$4,$5,$6)`, rec.ID, b.Code, b.DatasetID, b.VersionID, b.URI, b.SHA256); err != nil {
				return err
			}
			e, err := outbox.NewEvent("DATASET_VERSION", v.ID, "DatasetVersionCreated", map[string]any{"datasetVersionId": v.ID, "datasetId": v.DatasetID, "status": v.Status, "ingestionReceiptId": rec.ID})
			if err != nil {
				return err
			}
			if err = outbox.Append(ctx, tx, e); err != nil {
				return err
			}
			if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &op.Request.WorkspaceID, ActorType: "USER", ActorID: &op.Request.ActorID, Action: "DATASET_VERSION_READY", ObjectType: "DATASET_VERSION", ObjectID: v.ID, AfterState: b}); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO evidence_relation(evidence_id,object_type,object_id,relation_type) VALUES($1,'DATASET_VERSION',$2,'SUPPORTS')`, ev.ID, v.ID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE ingestion_operation SET state='ACCEPTED' WHERE id=$1`, id); err != nil {
			return err
		}
		return s.fact(ctx, tx, current, "IngestionAccepted", map[string]any{"receiptId": rec.ID, "manifestHash": hash})
	})
	slices.SortFunc(rec.Bindings, func(a, b d.Binding) int { return strings.Compare(a.Code, b.Code) })
	return rec, err
}
