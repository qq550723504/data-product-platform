package infrastructure

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
)

const attemptSelect = `SELECT a.id,a.attempt_no,a.workspace_id,a.publication_id,a.product_release_id,a.binding_id,a.identifier_id,a.provider,a.instance_ref,a.adapter_revision,a.external_subject_id,a.external_identifier,a.request_key,a.principal_ref,a.actor_id,a.evidence_id,a.created_at,COALESCE(r.outcome,'OPEN') FROM external_publication_attempt a LEFT JOIN external_publication_attempt_resolution r ON r.attempt_id=a.id`

func scanAttempt(row pgx.Row) (d.Attempt, error) {
	var a d.Attempt
	q := &a.Request
	err := row.Scan(&a.ID, &a.Number, &q.WorkspaceID, &q.PublicationID, &q.ReleaseID, &q.BindingID, &q.IdentifierID, &q.Provider, &q.Instance, &q.AdapterRevision, &q.ExternalSubjectID, &q.ExternalIdentifier, &q.RequestKey, &a.PrincipalRef, &a.ActorID, &a.EvidenceID, &a.CreatedAt, &a.Resolution)
	return a, Error(err)
}
func (r *Repository) LatestAttempt(ctx context.Context, q Querier, ws, pub uuid.UUID) (d.Attempt, error) {
	return scanAttempt(q.QueryRow(ctx, attemptSelect+` WHERE a.workspace_id=$1 AND a.publication_id=$2 ORDER BY a.attempt_no DESC LIMIT 1`, ws, pub))
}
func (r *Repository) Attempts(ctx context.Context, ws, pub uuid.UUID, limit, offset int) ([]d.Attempt, error) {
	rows, err := r.Pool.Query(ctx, attemptSelect+` WHERE a.workspace_id=$1 AND a.publication_id=$2 ORDER BY a.attempt_no DESC LIMIT $3 OFFSET $4`, ws, pub, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.Attempt{}
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *Repository) InsertAttempt(ctx context.Context, tx pgx.Tx, a d.Attempt) error {
	q := a.Request
	b, err := q.Bytes()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_publication_attempt(id,publication_id,workspace_id,attempt_no,product_release_id,binding_id,identifier_id,provider,instance_ref,adapter_revision,external_subject_id,external_identifier,request_key,request_bytes,request_hash,principal_ref,actor_id,evidence_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, a.ID, q.PublicationID, q.WorkspaceID, a.Number, q.ReleaseID, q.BindingID, q.IdentifierID, q.Provider, q.Instance, q.AdapterRevision, q.ExternalSubjectID, q.ExternalIdentifier, q.RequestKey, string(b), q.Hash(), a.PrincipalRef, a.ActorID, a.EvidenceID)
	return Error(err)
}
func (r *Repository) ReleaseIdentifier(ctx context.Context, tx pgx.Tx, ws uuid.UUID, provider string, release uuid.UUID) (d.Identifier, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT i.id FROM external_identifier i WHERE workspace_id=$1 AND provider=$2 AND object_type='PRODUCT_RELEASE' AND object_id=$3 AND NOT EXISTS(SELECT 1 FROM external_identifier_disposition x WHERE x.identifier_id=i.id)`, ws, provider, release).Scan(&id)
	if err != nil {
		return d.Identifier{}, Error(err)
	}
	return r.Identifier(ctx, tx, ws, id, true)
}

const callsSelect = `SELECT c.id,c.attempt_id,c.kind,c.created_at,EXISTS(SELECT 1 FROM external_provider_dispatch d WHERE d.call_id=c.id),o.id,COALESCE(o.instance_ref,''),COALESCE(o.request_key,''),COALESCE(o.request_hash,''),COALESCE(o.state,''),COALESCE(o.external_id,''),COALESCE(o.remote_id,''),COALESCE(o.error_code,'') FROM external_provider_call c LEFT JOIN external_provider_observation o ON o.call_id=c.id`

func (r *Repository) Calls(ctx context.Context, q Querier, ws, pub, attempt uuid.UUID, limit, offset int) ([]d.Call, error) {
	rows, err := q.Query(ctx, callsSelect+` JOIN external_publication_attempt a ON a.id=c.attempt_id WHERE a.workspace_id=$1 AND a.publication_id=$2 AND a.id=$3 ORDER BY c.created_at,c.id LIMIT $4 OFFSET $5`, ws, pub, attempt, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.Call{}
	for rows.Next() {
		var c d.Call
		v := &c.Result
		err = rows.Scan(&c.ID, &c.AttemptID, &c.Kind, &c.CreatedAt, &c.Dispatched, &c.ObservationID, &v.Instance, &v.RequestKey, &v.RequestHash, &v.State, &v.ExternalID, &v.RemoteID, &c.Code)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Recovery asks only for the newest catalog fact and verified preflight markers;
// it never depends on a bounded public history page to find durable claims.
func (r *Repository) RecoveryCalls(ctx context.Context, q Querier, attempt uuid.UUID) ([]d.Call, error) {
	rows, err := q.Query(ctx, callsSelect+` WHERE c.attempt_id=$1 AND c.id IN (SELECT DISTINCT ON(kind) id FROM external_provider_call WHERE attempt_id=$1 ORDER BY kind,created_at DESC,id DESC) ORDER BY c.created_at,c.id`, attempt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.Call{}
	for rows.Next() {
		var c d.Call
		v := &c.Result
		err = rows.Scan(&c.ID, &c.AttemptID, &c.Kind, &c.CreatedAt, &c.Dispatched, &c.ObservationID, &v.Instance, &v.RequestKey, &v.RequestHash, &v.State, &v.ExternalID, &v.RemoteID, &c.Code)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *Repository) Pending(ctx context.Context, providers []string, after uuid.UUID) ([]d.Publication, error) {
	rows, err := r.Pool.Query(ctx, publicationSelect+` p WHERE provider=ANY($1) AND status IN ('SUBMITTING','REGISTERED') AND NOT EXISTS(SELECT 1 FROM external_publication_attempt a JOIN external_publication_attempt_resolution x ON x.attempt_id=a.id WHERE a.publication_id=p.id AND a.attempt_no=(SELECT max(attempt_no) FROM external_publication_attempt WHERE publication_id=p.id)) ORDER BY (id>$2) DESC,id LIMIT 25`, providers, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.Publication{}
	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
