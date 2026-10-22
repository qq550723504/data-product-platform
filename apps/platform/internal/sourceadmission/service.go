package sourceadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	di "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	ri "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type Connection struct {
	ID              uuid.UUID          `json:"id"`
	WorkspaceID     uuid.UUID          `json:"workspaceId"`
	Namespace       uuid.UUID          `json:"namespace"`
	DatasetID       uuid.UUID          `json:"datasetId"`
	ResourceID      uuid.UUID          `json:"resourceId"`
	AuthorizationID uuid.UUID          `json:"authorizationId"`
	Project         string             `json:"project"`
	Organization    string             `json:"organization"`
	Mode            string             `json:"mode"`
	BaseURL         string             `json:"baseUrl"`
	TokenFile       string             `json:"tokenFile"`
	Consumer        string             `json:"consumer"`
	Purpose         string             `json:"purpose"`
	Scope           rd.NormalizedScope `json:"scope"`
}

func (c Connection) fingerprint() string { b, _ := json.Marshal(c); return Hash(b) }
func (c Connection) valid() bool {
	return validUUID(c.ID) && validUUID(c.WorkspaceID) && validUUID(c.Namespace) && validUUID(c.DatasetID) && validUUID(c.ResourceID) && decimal.MatchString(c.Project) && decimal.MatchString(c.Organization) && c.Mode == "fork_approved" && c.Consumer != "" && c.Purpose != "" && c.Scope.Ref != ""
}

type Request struct {
	WorkspaceID  uuid.UUID `json:"-"`
	DatasetID    uuid.UUID `json:"datasetId"`
	ConnectionID uuid.UUID `json:"connectionId"`
	ReleaseID    uuid.UUID `json:"releaseId"`
	SnapshotID   uuid.UUID `json:"snapshotId"`
	ManifestSHA  string    `json:"manifestSha256"`
	Key          string    `json:"-"`
}

func (r Request) valid() bool {
	return validUUID(r.WorkspaceID) && validUUID(r.DatasetID) && validUUID(r.ConnectionID) && validUUID(r.ReleaseID) && validUUID(r.SnapshotID) && digest.MatchString(r.ManifestSHA) && len(r.Key) > 0 && len(r.Key) <= 255 && strings.TrimSpace(r.Key) == r.Key
}

type Receipt struct {
	ID          uuid.UUID  `json:"operationId"`
	CaptureID   *uuid.UUID `json:"-"`
	VersionID   *uuid.UUID `json:"datasetVersionId"`
	State       string     `json:"state"`
	ManifestSHA string     `json:"manifestSha256"`
	RowCount    int        `json:"rowCount"`
	Gold        bool       `json:"goldCertified"`
	Published   bool       `json:"published"`
	ErrorCode   string     `json:"errorCode,omitempty"`
}
type Caller func(context.Context, uuid.UUID) (principal.RequestPrincipal, error)
type VersionStore interface {
	PutVersion(context.Context, dd.DatasetVersion, string, io.Reader, int64, string) (string, error)
	Get(context.Context, string) (io.ReadCloser, error)
	LockPublication(context.Context, pgx.Tx, uuid.UUID) error
	FinalizePublication(context.Context, pgx.Tx, dd.DatasetVersion) error
}
type Service struct {
	Pool     *pgxpool.Pool
	Tx       *transaction.Manager
	Datasets *di.PostgresRepository
	Load     func(context.Context, uuid.UUID) (Connection, error)
	Store    func(Connection) (VersionStore, error)
	Source   *HTTPSource
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool, Tx: transaction.NewManager(pool), Datasets: di.NewPostgresRepository(pool)}
}

// RegisterConnection is trusted composition only. Changed bindings require a new
// explicit identity; startup cannot overwrite a disabled or existing connection.
func (s *Service) RegisterConnection(ctx context.Context, c Connection) error {
	if !c.valid() {
		return ErrDenied
	}
	return s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := deliveryfence.Lock(ctx, tx, c.WorkspaceID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO source_admission_connection(id,workspace_id,namespace_id,project_id,organization_id,binding_sha256,mode) VALUES($1,$2,$3,$4,$5,$6,'fork_approved') ON CONFLICT(id) DO NOTHING`, c.ID, c.WorkspaceID, c.Namespace, c.Project, c.Organization, c.fingerprint())
		if e != nil {
			return e
		}
		var same bool
		e = tx.QueryRow(ctx, `SELECT workspace_id=$2 AND binding_sha256=$3 FROM source_admission_connection WHERE id=$1`, c.ID, c.WorkspaceID, c.fingerprint()).Scan(&same)
		if e != nil {
			return e
		}
		if !same {
			return ErrConflict
		}
		return nil
	})
}
func (s *Service) gate(ctx context.Context, tx pgx.Tx, r Request, caller Caller) (Connection, principal.RequestPrincipal, error) {
	var zero Connection
	var p principal.RequestPrincipal
	if s.Load == nil || s.Store == nil || s.Source == nil || caller == nil {
		return zero, p, ErrDenied
	}
	c, e := s.Load(ctx, r.ConnectionID)
	if e != nil || !c.valid() || c.WorkspaceID != r.WorkspaceID || c.DatasetID != r.DatasetID {
		return zero, p, ErrDenied
	}
	p, e = caller(ctx, r.WorkspaceID)
	if e != nil || p.ActorID == uuid.Nil || p.Subject == "" || p.WorkspaceID != r.WorkspaceID || !p.HasCapability(Capability) {
		return zero, p, ErrDenied
	}
	if _, e = deliveryfence.Lock(ctx, tx, r.WorkspaceID); e != nil {
		return zero, p, e
	}
	var ok bool
	e = tx.QueryRow(ctx, `SELECT enabled AND workspace_id=$2 AND binding_sha256=$3 AND mode='fork_approved' FROM source_admission_connection WHERE id=$1 FOR SHARE`, c.ID, c.WorkspaceID, c.fingerprint()).Scan(&ok)
	if e != nil || !ok {
		return zero, p, ErrDenied
	}
	e = tx.QueryRow(ctx, `SELECT workspace_id=$2 AND lifecycle_status NOT IN('BLOCKED','ARCHIVED') AND deleted_at IS NULL FROM data_resource WHERE id=$1 FOR SHARE`, c.ResourceID, c.WorkspaceID).Scan(&ok)
	if e != nil || !ok {
		return zero, p, ErrDenied
	}
	e = tx.QueryRow(ctx, `SELECT workspace_id=$2 AND dataset_type='RAW' AND source_resource_id=$3 AND lifecycle_status='ACTIVE' AND deleted_at IS NULL FROM dataset WHERE id=$1 FOR UPDATE`, c.DatasetID, c.WorkspaceID, c.ResourceID).Scan(&ok)
	if e != nil || !ok {
		return zero, p, ErrDenied
	}
	path := rd.EntitlementDirectUse
	if c.AuthorizationID != uuid.Nil {
		path = rd.EntitlementDownstream
	}
	for _, action := range []string{"READ", "PROCESS"} {
		decision, err := ri.NewPostgresRepository(s.Pool).CheckCurrentEntitlementTx(ctx, tx, rd.EntitlementRequest{WorkspaceID: c.WorkspaceID, AuthorizationID: c.AuthorizationID, DataResourceID: c.ResourceID, ConsumerRef: c.Consumer, Purpose: c.Purpose, Action: action, Scope: c.Scope, AsOf: time.Now().UTC(), Path: path})
		if err != nil || decision.Decision != rd.DecisionAllowed {
			return zero, p, ErrDenied
		}
	}
	return c, p, nil
}
func (s *Service) boundary(ctx context.Context, r Request, caller Caller) (Connection, principal.RequestPrincipal, error) {
	var c Connection
	var p principal.RequestPrincipal
	e := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		c, p, e = s.gate(ctx, tx, r, caller)
		return e
	})
	return c, p, e
}
func (s *Service) prepare(ctx context.Context, r Request, caller Caller) (Receipt, error) {
	var rec Receipt
	if !r.valid() {
		return rec, ErrConflict
	}
	e := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, p, e := s.gate(ctx, tx, r, caller)
		if e != nil {
			return e
		}
		// Workspace fence serializes key and snapshot reservation, without HTTP in tx.
		var old Request
		var subject string
		e = tx.QueryRow(ctx, `SELECT id,state,manifest_sha256,dataset_id,connection_id,release_id,snapshot_id,principal_ref,coalesce(error_code,'') FROM source_admission_operation WHERE workspace_id=$1 AND actor_id=$2 AND idempotency_key=$3 FOR UPDATE`, r.WorkspaceID, p.ActorID, r.Key).Scan(&rec.ID, &rec.State, &old.ManifestSHA, &old.DatasetID, &old.ConnectionID, &old.ReleaseID, &old.SnapshotID, &subject, &rec.ErrorCode)
		if e == nil {
			if old.ManifestSHA != r.ManifestSHA || old.DatasetID != r.DatasetID || old.ConnectionID != r.ConnectionID || old.ReleaseID != r.ReleaseID || old.SnapshotID != r.SnapshotID || subject != p.Subject {
				return ErrConflict
			}
			rec.ManifestSHA = r.ManifestSHA
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		rec.ID = uuid.New()
		rec.State = "PREPARED"
		rec.ManifestSHA = r.ManifestSHA
		_, e = tx.Exec(ctx, `INSERT INTO source_admission_operation(id,workspace_id,actor_id,principal_ref,idempotency_key,connection_id,dataset_id,release_id,snapshot_id,manifest_sha256,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PREPARED')`, rec.ID, r.WorkspaceID, p.ActorID, p.Subject, r.Key, r.ConnectionID, r.DatasetID, r.ReleaseID, r.SnapshotID, r.ManifestSHA)
		if e != nil {
			return e
		}
		return s.fact(ctx, tx, r, p, rec.ID, "SourceAdmissionPrepared", map[string]any{"operationId": rec.ID, "manifestSha256": r.ManifestSHA})
	})
	return rec, e
}
func (s *Service) fact(ctx context.Context, tx pgx.Tx, r Request, p principal.RequestPrincipal, id uuid.UUID, event string, payload any) error {
	e, err := outbox.NewEvent("SOURCE_ADMISSION", id, event, payload)
	if err != nil {
		return err
	}
	if err = outbox.Append(ctx, tx, e); err != nil {
		return err
	}
	if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &r.WorkspaceID, ActorType: "USER", ActorID: &p.ActorID, Action: event, ObjectType: "SOURCE_ADMISSION", ObjectID: id, AfterState: payload}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: r.WorkspaceID, EvidenceType: "SOURCE_ADMISSION", Title: event, SourceType: "SOURCE_ADMISSION", SourceID: &id, CreatedBy: &p.ActorID, Metadata: map[string]any{"fact": payload}}, evidence.Relation{ObjectType: "DATASET", ObjectID: r.DatasetID, RelationType: "SUPPORTS"})
	return err
}

// Dataset publication keeps the established Dataset aggregate and audit
// contract. Source capture/operation facts remain separate producer facts.
func (s *Service) datasetFact(ctx context.Context, tx pgx.Tx, r Request, p principal.RequestPrincipal, v dd.DatasetVersion, previous dd.VersionStatus, replacement *uuid.UUID, capture uuid.UUID) error {
	event, action, reason := "DatasetVersionCreated", "DATASET_VERSION_READY", ""
	if v.Status == dd.VersionSuperseded {
		event, action, reason = "DatasetVersionSuperseded", "DATASET_VERSION_SUPERSEDED", "a newer DatasetVersion became current"
	} else if v.Status != dd.VersionReady {
		return ErrIntegrity
	}
	payload := map[string]any{"datasetVersionId": v.ID, "datasetId": v.DatasetID, "versionNo": v.VersionNo, "previousStatus": previous, "status": v.Status, "checksum": v.ChecksumValue, "generatedByExecutionId": v.GeneratedByExecutionID, "generatedByEntityMatchJobId": v.GeneratedByEntityMatchJobID}
	if replacement != nil {
		payload["replacementVersionId"] = *replacement
	} else {
		payload["sourceCaptureId"] = capture
	}
	e, err := outbox.NewEvent("DATASET_VERSION", v.ID, event, payload)
	if err != nil {
		return err
	}
	if err = outbox.Append(ctx, tx, e); err != nil {
		return err
	}
	after := make(map[string]any, len(payload)+2)
	for k, value := range payload {
		after[k] = value
	}
	after["storageUri"], after["rowCount"] = v.StorageURI, v.RowCount
	if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &r.WorkspaceID, ActorType: "USER", ActorID: &p.ActorID, Action: action, ObjectType: "DATASET_VERSION", ObjectID: v.ID, BeforeState: map[string]any{"status": previous}, AfterState: after, Reason: reason}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: r.WorkspaceID, EvidenceType: "DATASET_VERSION", Title: event, SourceType: "DATASET_VERSION", SourceID: &v.ID, CreatedBy: &p.ActorID, Metadata: map[string]any{"fact": payload}}, evidence.Relation{ObjectType: "DATASET", ObjectID: r.DatasetID, RelationType: "SUPPORTS"})
	return err
}

func terminalAdmissionError(code string) error {
	switch code {
	case ErrPublicationConflict.Error():
		return ErrPublicationConflict
	case ErrConflict.Error():
		return ErrConflict
	case ErrUnsupported.Error():
		return ErrUnsupported
	default:
		return ErrIntegrity
	}
}

func (s *Service) Admit(ctx context.Context, r Request, caller Caller) (Receipt, error) {
	rec, err := s.prepare(ctx, r, caller)
	if err != nil {
		return rec, err
	}
	if rec.State == "FAILED_FINAL" {
		return rec, terminalAdmissionError(rec.ErrorCode)
	}
	// Only short workspace/parent-row fences cross transactions. Do not hold a
	// pooled connection/advisory lock across HTTP: blocked contenders could
	// otherwise exhaust the pool needed by the active copy's durable journal.
	err = func(ctx context.Context) error {
		c, _, e := s.boundary(ctx, r, caller)
		if e != nil {
			return e
		}
		blob, e := s.Source.Fetch(ctx, s, rec.ID, r, c, "manifest.json", "")
		if e != nil {
			return e
		}
		manifest, e := ValidateManifest(blob, c, r)
		if e != nil {
			return e
		}
		if e = validateDeclaredBundleBudget(blob, manifest); e != nil {
			return e
		}
		blobs := map[string][]byte{"manifest.json": blob}
		for _, a := range manifest.Artifacts {
			fresh, _, e := s.boundary(ctx, r, caller)
			if e != nil {
				return e
			}
			if fresh.fingerprint() != c.fingerprint() {
				return ErrDenied
			}
			b, e := s.Source.Fetch(ctx, s, rec.ID, r, c, a.Name, a.ID)
			if e != nil {
				return e
			}
			blobs[a.Name] = b
		}
		if e = ValidateBundle(manifest, blobs); e != nil {
			return e
		}
		var capture uuid.UUID
		var v dd.DatasetVersion
		var state string
		e = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, _, e := s.gate(ctx, tx, r, caller)
			if e != nil {
				return e
			}
			var expected, content string
			var release uuid.UUID
			e = tx.QueryRow(ctx, `SELECT id,version_id,state,manifest_sha256,content_digest,release_id FROM source_admission_capture WHERE dataset_id=$1 AND connection_id=$2 AND snapshot_id=$3 FOR UPDATE`, r.DatasetID, r.ConnectionID, r.SnapshotID).Scan(&capture, &v.ID, &state, &expected, &content, &release)
			if e == nil {
				if expected != r.ManifestSHA || content != manifest.ContentDigest || release != r.ReleaseID {
					return ErrConflict
				}
				v, e = s.Datasets.GetVersionTx(ctx, tx, v.ID)
				if e == nil && ((state == "ACCEPTED_SOURCE" && v.Status != dd.VersionReady && v.Status != dd.VersionSuperseded) || (state == "PREPARED" && v.Status != dd.VersionCreated)) {
					return ErrDenied
				}
				return e
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			_, p, e := s.gate(ctx, tx, r, caller)
			if e != nil {
				return e
			}
			v, _, e = s.Datasets.AllocateVersion(ctx, tx, r.DatasetID, &p.ActorID, nil, nil)
			if e != nil {
				return e
			}
			capture = uuid.New()
			state = "PREPARED"
			_, e = tx.Exec(ctx, `INSERT INTO source_admission_capture(id,workspace_id,connection_id,dataset_id,version_id,release_id,snapshot_id,manifest_sha256,content_digest,member_count,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PREPARED')`, capture, r.WorkspaceID, r.ConnectionID, r.DatasetID, v.ID, r.ReleaseID, r.SnapshotID, r.ManifestSHA, manifest.ContentDigest, len(manifest.Content.Items))
			if e != nil {
				return e
			}
			for _, name := range []string{"manifest.json", "records.json", "records.jsonl"} {
				b := blobs[name]
				if _, e = tx.Exec(ctx, `INSERT INTO source_admission_asset(capture_id,name,sha256,byte_size) VALUES($1,$2,$3,$4)`, capture, name, Hash(b), len(b)); e != nil {
					return e
				}
			}
			return nil
		})
		if e != nil {
			return e
		}
		store, e := s.Store(c)
		if e != nil || store == nil {
			return ErrDenied
		}
		var primaryURI string
		for _, name := range []string{"manifest.json", "records.json", "records.jsonl"} {
			if _, _, e = s.boundary(ctx, r, caller); e != nil {
				return e
			}
			var uri *string
			var hash string
			var size int64
			e = s.Pool.QueryRow(ctx, `SELECT storage_uri,sha256,byte_size FROM source_admission_asset WHERE capture_id=$1 AND name=$2`, capture, name).Scan(&uri, &hash, &size)
			if e != nil {
				return ErrIntegrity
			}
			if hash != Hash(blobs[name]) || size != int64(len(blobs[name])) {
				return ErrIntegrity
			}
			if uri == nil {
				if state == "ACCEPTED_SOURCE" {
					return ErrIntegrity
				}
				key := fmt.Sprintf("core/%s/datasets/%s/v%06d/source/%s/%s", r.WorkspaceID, r.DatasetID, v.VersionNo, capture, name)
				contentType := "application/json"
				if name == "records.jsonl" {
					contentType = "application/x-ndjson"
				}
				loc, e := store.PutVersion(ctx, v, key, bytes.NewReader(blobs[name]), size, contentType)
				if e != nil {
					return e
				}
				uri = &loc
				if e = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
					if _, _, e := s.gate(ctx, tx, r, caller); e != nil {
						return e
					}
					_, e := tx.Exec(ctx, `UPDATE source_admission_asset SET storage_uri=$3 WHERE capture_id=$1 AND name=$2 AND storage_uri IS NULL`, capture, name, loc)
					return e
				}); e != nil {
					return e
				}
			}
			// Existing accepted copies are never overwritten or repaired.
			actual, e := store.Get(ctx, *uri)
			if e != nil {
				if errors.Is(e, ErrIntegrity) || errors.Is(e, lc.ErrStorageNotFound) {
					return ErrIntegrity
				}
				if errors.Is(e, lc.ErrStorageDenied) || errors.Is(e, lc.ErrStorageScope) {
					return ErrDenied
				}
				return ErrUnknown
			}
			data, e := io.ReadAll(io.LimitReader(actual, MaxBytes+1))
			closeErr := actual.Close()
			if e != nil || closeErr != nil {
				return ErrUnknown
			}
			if int64(len(data)) != size || Hash(data) != hash {
				return ErrIntegrity
			}
			if name == "records.jsonl" {
				primaryURI = *uri
			}
		}
		return s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, p, e := s.gate(ctx, tx, r, caller)
			if e != nil {
				return e
			}
			if e = tx.QueryRow(ctx, `SELECT state FROM source_admission_capture WHERE id=$1 FOR UPDATE`, capture).Scan(&state); e != nil {
				return e
			}
			if state != "ACCEPTED_SOURCE" {
				if e = store.LockPublication(ctx, tx, v.ID); e != nil {
					return e
				}
				for _, i := range manifest.Content.Items {
					if _, e = tx.Exec(ctx, `INSERT INTO source_admission_identity(connection_id,namespace_id,source_id,record_sha256) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, r.ConnectionID, c.Namespace, i.Source.ID, i.RecordHash); e != nil {
						return e
					}
					var saved string
					if e = tx.QueryRow(ctx, `SELECT record_sha256 FROM source_admission_identity WHERE namespace_id=$1 AND source_id=$2`, c.Namespace, i.Source.ID).Scan(&saved); e != nil {
						return e
					}
					if saved != i.RecordHash {
						return ErrConflict
					}
					if _, e = tx.Exec(ctx, `INSERT INTO source_admission_member(capture_id,connection_id,source_id,assignment_id,task_id,revision,result_sha256,record_sha256,namespace_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, capture, r.ConnectionID, i.Source.ID, i.Assignment, i.Task, i.Revision, i.ResultHash, i.RecordHash, c.Namespace); e != nil {
						return e
					}
				}
				if e = v.MarkReady("OBJECT_STORAGE", primaryURI, "application/x-ndjson", "SHA256", Hash(blobs["records.jsonl"]), int64(len(manifest.Content.Items)), int64(len(blobs["records.jsonl"]))); e != nil {
					return e
				}
				previous, e := s.Datasets.SetReady(ctx, tx, v)
				if errors.Is(e, dd.ErrStaleVersionRecovery) {
					// Keep the existing monotonic current-version rule. This reserved
					// version can never publish; persist a terminal original-key result.
					return ErrPublicationConflict
				}
				if e != nil {
					return e
				}
				if e = store.FinalizePublication(ctx, tx, v); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `UPDATE source_admission_capture SET state='ACCEPTED_SOURCE' WHERE id=$1`, capture); e != nil {
					return e
				}
				if e = s.fact(ctx, tx, r, p, capture, "SourceAdmissionAccepted", map[string]any{"captureId": capture, "datasetVersionId": v.ID, "manifestSha256": r.ManifestSHA, "contentDigest": manifest.ContentDigest, "rowCount": len(manifest.Content.Items)}); e != nil {
					return e
				}
				if previous != nil {
					if e = s.datasetFact(ctx, tx, r, p, *previous, dd.VersionReady, &v.ID, capture); e != nil {
						return e
					}
				}
				if e = s.datasetFact(ctx, tx, r, p, v, dd.VersionCreated, nil, capture); e != nil {
					return e
				}
			} else {
				// DB closure is revalidated on every receipt replay, not just on creation.
				var ok bool
				e = tx.QueryRow(ctx, `SELECT count(*)=$2 FROM source_admission_member WHERE capture_id=$1`, capture, len(manifest.Content.Items)).Scan(&ok)
				if e != nil || !ok {
					return ErrIntegrity
				}
				v, e = s.Datasets.GetVersionTx(ctx, tx, v.ID)
				if e != nil {
					return e
				}
				if v.Status != dd.VersionReady && v.Status != dd.VersionSuperseded {
					return ErrDenied
				}
				if v.ChecksumValue != Hash(blobs["records.jsonl"]) || v.StorageURI != primaryURI || v.RowCount == nil || *v.RowCount != int64(len(manifest.Content.Items)) {
					return ErrIntegrity
				}
				for _, i := range manifest.Content.Items {
					var same bool
					e = tx.QueryRow(ctx, `SELECT m.task_id=$3 AND m.assignment_id=$4 AND m.revision=$5 AND m.result_sha256=$6 AND m.record_sha256=$7 AND x.record_sha256=$7 FROM source_admission_member m JOIN source_admission_identity x ON x.namespace_id=m.namespace_id AND x.source_id=m.source_id WHERE m.capture_id=$1 AND m.source_id=$2 AND m.connection_id=$8 AND m.namespace_id=$9`, capture, i.Source.ID, i.Task, i.Assignment, i.Revision, i.ResultHash, i.RecordHash, r.ConnectionID, c.Namespace).Scan(&same)
					if e != nil || !same {
						return ErrIntegrity
					}
				}
			}
			var opState string
			if e = tx.QueryRow(ctx, `SELECT state FROM source_admission_operation WHERE id=$1 FOR UPDATE`, rec.ID).Scan(&opState); e != nil {
				return e
			}
			if opState != "ACCEPTED_SOURCE" {
				if _, e = tx.Exec(ctx, `UPDATE source_admission_operation SET state='ACCEPTED_SOURCE',capture_id=$2 WHERE id=$1`, rec.ID, capture); e != nil {
					return e
				}
				if e = s.fact(ctx, tx, r, p, rec.ID, "SourceAdmissionRecorded", map[string]any{"operationId": rec.ID, "captureId": capture, "datasetVersionId": v.ID}); e != nil {
					return e
				}
			}
			rec.State = "ACCEPTED_SOURCE"
			rec.VersionID = &v.ID
			rec.CaptureID = &capture
			rec.RowCount = len(manifest.Content.Items)
			return nil
		})
	}(ctx)
	if errors.Is(err, ErrIntegrity) || errors.Is(err, ErrUnsupported) || errors.Is(err, ErrConflict) || errors.Is(err, ErrPublicationConflict) {
		original := err
		if e := s.Tx.Do(context.WithoutCancel(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, p, e := s.gate(ctx, tx, r, caller)
			if e != nil {
				return e
			}
			var state, code string
			if e = tx.QueryRow(ctx, `SELECT state,coalesce(error_code,'') FROM source_admission_operation WHERE id=$1 FOR UPDATE`, rec.ID).Scan(&state, &code); e != nil {
				return e
			}
			if state == "FAILED_FINAL" {
				// A same-key contender may have committed while this call read
				// the source. Return the frozen outcome after fresh authorization,
				// preserving this call's own transport observation and cost.
				rec.State, rec.ErrorCode = state, code
				return nil
			}
			if state != "PREPARED" {
				return nil
			}
			if _, e = tx.Exec(ctx, `UPDATE source_admission_operation SET state='FAILED_FINAL',error_code=$2 WHERE id=$1`, rec.ID, original.Error()); e != nil {
				return e
			}
			rec.State = "FAILED_FINAL"
			rec.ErrorCode = original.Error()
			return s.fact(ctx, tx, r, p, rec.ID, "SourceAdmissionRejected", map[string]any{"operationId": rec.ID, "code": original.Error()})
		}); e != nil {
			return rec, errors.Join(original, e)
		}
		if rec.State == "FAILED_FINAL" {
			return rec, terminalAdmissionError(rec.ErrorCode)
		}
	}
	return rec, err
}

// Lookup resolves a known operation without creating anything or reading bytes.
// PREPARED means unresolved; it never proves that a previous request did not run.
func (s *Service) Lookup(ctx context.Context, ws, id uuid.UUID, caller Caller) (Receipt, error) {
	var r Request
	var rec Receipt
	var actor uuid.UUID
	var subject string
	e := s.Pool.QueryRow(ctx, `SELECT workspace_id,dataset_id,connection_id,release_id,snapshot_id,manifest_sha256,actor_id,principal_ref FROM source_admission_operation WHERE id=$1 AND workspace_id=$2`, id, ws).Scan(&r.WorkspaceID, &r.DatasetID, &r.ConnectionID, &r.ReleaseID, &r.SnapshotID, &r.ManifestSHA, &actor, &subject)
	if e != nil {
		return rec, ErrDenied
	}
	e = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, p, e := s.gate(ctx, tx, r, caller)
		if e != nil || p.ActorID != actor || p.Subject != subject {
			return ErrDenied
		}
		return tx.QueryRow(ctx, `SELECT o.id,o.state,o.manifest_sha256,c.version_id,COALESCE(c.member_count,0),COALESCE(o.error_code,'') FROM source_admission_operation o LEFT JOIN source_admission_capture c ON c.id=o.capture_id WHERE o.id=$1`, id).Scan(&rec.ID, &rec.State, &rec.ManifestSHA, &rec.VersionID, &rec.RowCount, &rec.ErrorCode)
	})
	return rec, e
}

func (s *Service) LookupKey(ctx context.Context, ws uuid.UUID, key string, caller Caller) (Receipt, error) {
	if caller == nil || key == "" || len(key) > 255 {
		return Receipt{}, ErrDenied
	}
	p, e := caller(ctx, ws)
	if e != nil || p.WorkspaceID != ws || !p.HasCapability(Capability) {
		return Receipt{}, ErrDenied
	}
	var id uuid.UUID
	e = s.Pool.QueryRow(ctx, `SELECT id FROM source_admission_operation WHERE workspace_id=$1 AND actor_id=$2 AND principal_ref=$3 AND idempotency_key=$4`, ws, p.ActorID, p.Subject, key).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return Receipt{}, ErrUnknown
	}
	if e != nil {
		return Receipt{}, e
	}
	return s.Lookup(ctx, ws, id, caller)
}
