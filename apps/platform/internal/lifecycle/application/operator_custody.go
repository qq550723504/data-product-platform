package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	di "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

var (
	ErrCustodyScope       = errors.New("operator custody scope refused")
	ErrCustodyInterrupted = errors.New("operator custody incomplete/unknown; no automatic retry")
)

type CustodyTarget struct {
	VersionID uuid.UUID `json:"versionId"`
	URI       string    `json:"uri"`
	SHA256    string    `json:"sha256"`
	Bytes     int64     `json:"bytes"`
}

func (t CustodyTarget) Key() string { return strings.TrimPrefix(t.URI, "s3://raw/") }
func (t CustodyTarget) Validate() error {
	u, err := url.Parse(t.URI)
	key := t.Key()
	if err != nil || t.VersionID == uuid.Nil || u.Scheme != "s3" || u.Host != "raw" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
		!strings.HasPrefix(key, "datasets/") || path.Clean(key) != key || strings.ContainsAny(key, "%\\\x00\r\n?#") || !custodyHash(t.SHA256) || t.Bytes <= 0 || t.Bytes > 128<<20 {
		return ErrCustodyScope
	}
	return nil
}

// CustodyPlan is private operator composition. No HTTP principal or client can
// supply it. First is STANDARDIZED, Second CURATED; order is part of identity.
type CustodyPlan struct {
	WorkspaceID     uuid.UUID     `json:"workspaceId"`
	ActorID         uuid.UUID     `json:"actorId"`
	PrincipalRef    string        `json:"principalRef"`
	RunID           string        `json:"runId"`
	ProviderWitness string        `json:"providerWitness"`
	Endpoint        string        `json:"endpoint"`
	OutputDirectory string        `json:"outputDirectory"`
	PlanSHA256      string        `json:"planSha256"`
	ExpiresAt       time.Time     `json:"expiresAt"`
	First           CustodyTarget `json:"standardized"`
	Second          CustodyTarget `json:"curated"`
}

func custodyHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == strings.ToLower(s)
}
func (p CustodyPlan) Validate() error {
	host, port, err := net.SplitHostPort(p.Endpoint)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || host != "127.0.0.1" || portErr != nil || portNumber < 1 || portNumber > 65535 || strconv.Itoa(portNumber) != port || p.WorkspaceID == uuid.Nil || p.ActorID == uuid.Nil ||
		!validText(p.PrincipalRef, 255) || !validText(p.RunID, 128) || !validText(p.ProviderWitness, 255) || p.OutputDirectory == "" ||
		!custodyHash(p.PlanSHA256) || p.ExpiresAt.IsZero() || p.First.Validate() != nil || p.Second.Validate() != nil || p.First.VersionID == p.Second.VersionID || p.First.URI == p.Second.URI {
		return ErrCustodyScope
	}
	return nil
}

type CustodyObservation struct {
	CallID                uuid.UUID
	InvokedAt, ObservedAt time.Time
	InvocationCertainty   string
	HTTPStatus            int
	Bytes                 int64
	SHA256                string
	EOF, CloseOK          bool
	Failure               string
}
type CustodyJournal interface {
	StartCustodyCall(context.Context, int) (uuid.UUID, error)
	ObserveCustodyCall(context.Context, CustodyObservation) error
}
type CustodyReader interface {
	ReadCustody(context.Context, int, CustodyTarget, CustodyJournal) ([]byte, error)
}

type OperatorCustody struct {
	pool *pgxpool.Pool
	tx   *transaction.Manager
	plan CustodyPlan
	id   uuid.UUID
}

func NewOperatorCustody(pool *pgxpool.Pool, plan CustodyPlan) (*OperatorCustody, error) {
	if pool == nil || plan.Validate() != nil {
		return nil, ErrCustodyScope
	}
	return &OperatorCustody{pool: pool, tx: transaction.NewManager(pool), plan: plan}, nil
}
func (s *OperatorCustody) WithOwner(ctx context.Context, fn func(context.Context) error) error {
	return s.tx.WithAdvisoryLock(ctx, "operator-custody:"+s.plan.WorkspaceID.String(), fn)
}
func (s *OperatorCustody) caller() principal.RequestPrincipal {
	return principal.RequestPrincipal{WorkspaceID: s.plan.WorkspaceID, ActorID: s.plan.ActorID, Subject: s.plan.PrincipalRef}
}
func (s *OperatorCustody) facts(ctx context.Context, tx pgx.Tx, event, subject string, id uuid.UUID, rev int64, at time.Time, fields map[string]any) error {
	return appendLifecycleFacts(ctx, tx, "OPERATOR_CUSTODY", event, subject, id, s.caller(), rev, at, fields)
}
func (s *OperatorCustody) gate(ctx context.Context, tx pgx.Tx) error {
	if !time.Now().UTC().Before(s.plan.ExpiresAt) {
		return ErrStorageDenied
	}
	// Reuse the same admission semantics without constructing a protected
	// identity for the old prefix-external object.
	repo := di.NewPostgresRepository(s.pool)
	for _, target := range []CustodyTarget{s.plan.First, s.plan.Second} {
		v, err := validateRetainedVersion(ctx, tx, repo, s.plan.WorkspaceID, target.VersionID, false)
		if err != nil {
			return err
		}
		if v.StorageURI != target.URI || v.ChecksumAlgorithm != "SHA256" || v.ChecksumValue != target.SHA256 || v.ByteSize == nil || *v.ByteSize != target.Bytes {
			return ErrCustodyScope
		}
		if err = validateRetainedSources(ctx, tx, repo, s.plan.WorkspaceID, s.plan.ExpiresAt, v); err != nil {
			return err
		}
	}
	return nil
}

// Prepare commits the entire immutable two-member scope. Existing incomplete
// keys always refuse, including start-only interruption: never resume I/O.
func (s *OperatorCustody) Prepare(ctx context.Context, key string) (bool, error) {
	if !validText(key, 128) || s.id != uuid.Nil {
		return false, ErrCustodyScope
	}
	body, _ := json.Marshal(s.plan)
	digest := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(digest[:])
	replay := false
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, s.plan.WorkspaceID)
		if err != nil {
			return err
		}
		var prior string
		err = tx.QueryRow(ctx, `SELECT id,request_hash FROM operator_custody_acquisition WHERE workspace_id=$1 AND request_key=$2`, s.plan.WorkspaceID, key).Scan(&s.id, &prior)
		if err == nil {
			if prior != fingerprint {
				return ErrIdempotencyConflict
			}
			var accepted bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM operator_custody_termination WHERE acquisition_id=$1 AND outcome='ACCEPTED')`, s.id).Scan(&accepted)
			if err != nil {
				return err
			}
			if !accepted {
				return ErrCustodyInterrupted
			}
			replay = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = s.gate(ctx, tx); err != nil {
			return err
		}
		at := time.Now().UTC().Truncate(time.Microsecond)
		if s.plan.ExpiresAt.After(at.Add(time.Hour)) {
			return ErrCustodyScope
		}
		s.id = uuid.New()
		p := s.plan
		_, err = tx.Exec(ctx, `INSERT INTO operator_custody_acquisition(id,workspace_id,actor_id,principal_ref,run_id,request_key,request_hash,plan_sha256,provider_witness,endpoint,output_directory,first_version_id,first_uri,first_sha256,first_bytes,second_version_id,second_uri,second_sha256,second_bytes,prepared_at,expires_at,dependency_revision)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`, s.id, p.WorkspaceID, p.ActorID, p.PrincipalRef, p.RunID, key, fingerprint, p.PlanSHA256, p.ProviderWitness, p.Endpoint, p.OutputDirectory, p.First.VersionID, p.First.URI, p.First.SHA256, p.First.Bytes, p.Second.VersionID, p.Second.URI, p.Second.SHA256, p.Second.Bytes, at, p.ExpiresAt, rev)
		if err != nil {
			return err
		}
		return s.facts(ctx, tx, "OperatorCustodyPrepared", "OPERATOR_CUSTODY_ACQUISITION", s.id, rev, at, map[string]any{"plan_sha256": p.PlanSHA256, "request_hash": fingerprint, "run_id": p.RunID})
	})
	if err != nil {
		s.id = uuid.Nil
	}
	return replay, err
}

func (s *OperatorCustody) ID() uuid.UUID                  { return s.id }
func (s *OperatorCustody) MatchesPlan(p CustodyPlan) bool { return s.plan == p }
func (s *OperatorCustody) StartCustodyCall(ctx context.Context, member int) (uuid.UUID, error) {
	id := uuid.New()
	if s.id == uuid.Nil || (member != 1 && member != 2) {
		return uuid.Nil, ErrCustodyScope
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, s.plan.WorkspaceID)
		if err != nil {
			return err
		}
		if err = s.gate(ctx, tx); err != nil {
			return err
		}
		at := time.Now().UTC().Truncate(time.Microsecond)
		_, err = tx.Exec(ctx, `INSERT INTO operator_custody_call(id,workspace_id,acquisition_id,member_no,started_at,dependency_revision) VALUES($1,$2,$3,$4,$5,$6)`, id, s.plan.WorkspaceID, s.id, member, at, rev)
		if err != nil {
			return err
		}
		return s.facts(ctx, tx, "OperatorCustodyCallStarted", "OPERATOR_CUSTODY_CALL", id, rev, at, map[string]any{"acquisition_id": s.id, "member_no": member, "operation": "GET"})
	})
	return id, err
}

func (s *OperatorCustody) ObserveCustodyCall(ctx context.Context, o CustodyObservation) error {
	if s.id == uuid.Nil || o.CallID == uuid.Nil || o.InvokedAt.IsZero() || o.ObservedAt.Before(o.InvokedAt) {
		return ErrCustodyScope
	}
	o.InvokedAt = o.InvokedAt.UTC().Truncate(time.Microsecond)
	o.ObservedAt = o.ObservedAt.UTC().Truncate(time.Microsecond)
	b, _ := json.Marshal(o)
	hash := sha256.Sum256(b)
	fingerprint := hex.EncodeToString(hash[:])
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, s.plan.WorkspaceID)
		if err != nil {
			return err
		}
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM operator_custody_call WHERE id=$1 AND acquisition_id=$2 AND workspace_id=$3)`, o.CallID, s.id, s.plan.WorkspaceID).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return ErrCustodyScope
		}
		var old string
		err = tx.QueryRow(ctx, `SELECT observation_sha256 FROM operator_custody_observation WHERE call_id=$1`, o.CallID).Scan(&old)
		if err == nil {
			if old != fingerprint {
				return ErrIdempotencyConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO operator_custody_observation(id,workspace_id,call_id,invoked_at,observed_at,invocation_certainty,http_status,byte_size,body_sha256,eof,close_ok,failure,observation_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, o.CallID, s.plan.WorkspaceID, o.CallID, o.InvokedAt, o.ObservedAt, o.InvocationCertainty, o.HTTPStatus, o.Bytes, o.SHA256, o.EOF, o.CloseOK, o.Failure, fingerprint)
		if err != nil {
			return err
		}
		if err = cost.AppendOperatorCustodyActivity(ctx, tx, s.plan.WorkspaceID, o.CallID, o.InvokedAt); err != nil {
			return err
		}
		return s.facts(ctx, tx, "OperatorCustodyCallObserved", "OPERATOR_CUSTODY_OBSERVATION", o.CallID, rev, o.ObservedAt, map[string]any{"call_id": o.CallID, "observation_sha256": fingerprint, "invocation_certainty": o.InvocationCertainty, "failure": o.Failure, "http_status": o.HTTPStatus, "byte_size": o.Bytes, "body_sha256": o.SHA256})
	})
}

// Finish is called only after the synchronous reader has returned and closed.
// A missing observation prevents termination; crash recovery cannot infer stop.
func (s *OperatorCustody) Finish(ctx context.Context, accepted bool, manifestHash string) error {
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, s.plan.WorkspaceID)
		if err != nil {
			return err
		}
		outcome := "FAILED"
		if accepted {
			outcome = "ACCEPTED"
			if !custodyHash(manifestHash) {
				return ErrCustodyScope
			}
			if err = s.gate(ctx, tx); err != nil {
				return err
			}
		} else if manifestHash != "" {
			return ErrCustodyScope
		}
		id := uuid.New()
		at := time.Now().UTC().Truncate(time.Microsecond)
		_, err = tx.Exec(ctx, `INSERT INTO operator_custody_termination(id,workspace_id,acquisition_id,outcome,owner_verified_quiescent,manifest_sha256,occurred_at,dependency_revision) VALUES($1,$2,$3,$4,true,$5,$6,$7)`, id, s.plan.WorkspaceID, s.id, outcome, manifestHash, at, rev)
		if err != nil {
			return err
		}
		return s.facts(ctx, tx, "OperatorCustodyTerminated", "OPERATOR_CUSTODY_TERMINATION", id, rev, at, map[string]any{"acquisition_id": s.id, "outcome": outcome, "manifest_sha256": manifestHash, "owner_verified_quiescent": true})
	})
}

func (s *OperatorCustody) AcceptedManifest(ctx context.Context) (string, error) {
	var h string
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT manifest_sha256 FROM operator_custody_termination WHERE workspace_id=$1 AND acquisition_id=$2 AND outcome='ACCEPTED'`, s.plan.WorkspaceID, s.id).Scan(&h)
	})
	return h, err
}
