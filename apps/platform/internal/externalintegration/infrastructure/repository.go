package infrastructure

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
)

type Repository struct{ Pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{Pool: pool} }

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func Error(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return d.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return d.ErrConflict
		}
	}
	return err
}

const bindingSelect = `SELECT b.id,b.workspace_id,b.provider,b.external_subject_id,b.basis_evidence_id,b.subject_type,b.subject_id,b.principal_ref,b.evidence_id,
 CASE WHEN EXISTS(SELECT 1 FROM external_identity_binding_disposition x WHERE x.binding_id=b.id) THEN 'INVALIDATED' ELSE 'BOUND' END,b.created_by,b.created_at FROM external_identity_binding b`

func scanBinding(row pgx.Row) (d.Binding, error) {
	var b d.Binding
	err := row.Scan(&b.ID, &b.WorkspaceID, &b.Provider, &b.ExternalSubjectID, &b.BasisEvidenceID, &b.SubjectType, &b.SubjectID, &b.PrincipalRef, &b.EvidenceID, &b.Status, &b.CreatedBy, &b.CreatedAt)
	return b, Error(err)
}
func (r *Repository) Binding(ctx context.Context, q Querier, ws, id uuid.UUID, lock bool) (d.Binding, error) {
	sql := bindingSelect + ` WHERE b.workspace_id=$1 AND b.id=$2`
	if lock {
		sql += ` FOR UPDATE OF b`
	}
	b, err := scanBinding(q.QueryRow(ctx, sql, ws, id))
	if err == nil && lock {
		// A disposition may commit while this statement waits for the parent.
		// Read it again after acquiring the lock using a fresh RC snapshot.
		var invalidated bool
		err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=$1)`, id).Scan(&invalidated)
		if invalidated {
			b.Status = "INVALIDATED"
		}
	}
	return b, err
}
func (r *Repository) Bindings(ctx context.Context, ws uuid.UUID, limit, offset int) ([]d.Binding, error) {
	rows, err := r.Pool.Query(ctx, bindingSelect+` WHERE b.workspace_id=$1 ORDER BY b.created_at,b.id LIMIT $2 OFFSET $3`, ws, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []d.Binding{}
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return items, rows.Err()
}
func (r *Repository) InsertBinding(ctx context.Context, tx pgx.Tx, b d.Binding) error {
	_, err := tx.Exec(ctx, `INSERT INTO external_identity_binding(id,workspace_id,provider,subject_type,subject_id,principal_ref,external_subject_id,basis_evidence_id,evidence_id,created_by,created_at) VALUES($1,$2,$3,'PRINCIPAL',$4,$5,$6,$7,$8,$4,$9)`, b.ID, b.WorkspaceID, b.Provider, b.SubjectID, b.PrincipalRef, b.ExternalSubjectID, b.BasisEvidenceID, b.EvidenceID, b.CreatedAt)
	return Error(err)
}

const identifierSelect = `SELECT i.id,i.workspace_id,i.provider,i.object_type,i.object_id,i.subject_binding_id,i.external_id,i.basis_evidence_id,i.principal_ref,i.evidence_id,
 CASE WHEN EXISTS(SELECT 1 FROM external_identifier_disposition x WHERE x.identifier_id=i.id) THEN 'INVALIDATED' ELSE 'RECORDED' END,i.created_by,i.created_at FROM external_identifier i`

func scanIdentifier(row pgx.Row) (d.Identifier, error) {
	var i d.Identifier
	err := row.Scan(&i.ID, &i.WorkspaceID, &i.Provider, &i.ObjectType, &i.ObjectID, &i.SubjectBindingID, &i.ExternalID, &i.BasisEvidenceID, &i.PrincipalRef, &i.EvidenceID, &i.Status, &i.CreatedBy, &i.CreatedAt)
	return i, Error(err)
}
func (r *Repository) Identifier(ctx context.Context, q Querier, ws, id uuid.UUID, lock bool) (d.Identifier, error) {
	sql := identifierSelect + ` WHERE i.workspace_id=$1 AND i.id=$2`
	if lock {
		sql += ` FOR UPDATE OF i`
	}
	i, err := scanIdentifier(q.QueryRow(ctx, sql, ws, id))
	if err == nil && lock {
		var invalid bool
		err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_identifier_disposition WHERE identifier_id=$1)`, id).Scan(&invalid)
		if invalid {
			i.Status = "INVALIDATED"
		}
	}
	return i, err
}
func (r *Repository) Identifiers(ctx context.Context, ws uuid.UUID, limit, offset int) ([]d.Identifier, error) {
	rows, err := r.Pool.Query(ctx, identifierSelect+` WHERE i.workspace_id=$1 ORDER BY i.created_at,i.id LIMIT $2 OFFSET $3`, ws, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []d.Identifier{}
	for rows.Next() {
		i, err := scanIdentifier(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}
func (r *Repository) InsertIdentifier(ctx context.Context, tx pgx.Tx, i d.Identifier) error {
	var subject, resource, product, release *uuid.UUID
	switch i.ObjectType {
	case "SUBJECT":
		subject = &i.ObjectID
	case "DATA_RESOURCE":
		resource = &i.ObjectID
	case "DATA_PRODUCT":
		product = &i.ObjectID
	case "PRODUCT_RELEASE":
		release = &i.ObjectID
	default:
		return d.ErrInvalid
	}
	_, err := tx.Exec(ctx, `INSERT INTO external_identifier(id,workspace_id,provider,object_type,subject_binding_id,subject_id,resource_id,product_id,release_id,external_id,basis_evidence_id,evidence_id,principal_ref,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, i.ID, i.WorkspaceID, i.Provider, i.ObjectType, i.SubjectBindingID, subject, resource, product, release, i.ExternalID, i.BasisEvidenceID, i.EvidenceID, i.PrincipalRef, i.CreatedBy, i.CreatedAt)
	return Error(err)
}
func (r *Repository) Basis(ctx context.Context, tx pgx.Tx, ws, id uuid.UUID) error {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence WHERE id=$1 AND workspace_id=$2)`, id, ws).Scan(&found)
	if err != nil {
		return err
	}
	if !found {
		return d.ErrNotFound
	}
	return nil
}

// Release is read only; its PUBLISHED history is frozen by the Core guards.
func (r *Repository) Release(ctx context.Context, tx pgx.Tx, ws, id uuid.UUID, published bool) error {
	var state string
	err := tx.QueryRow(ctx, `SELECT r.status FROM product_release r JOIN data_product p ON p.id=r.product_id WHERE r.id=$1 AND p.workspace_id=$2 FOR SHARE OF r`, id, ws).Scan(&state)
	if err != nil {
		return Error(err)
	}
	if published && state != "PUBLISHED" {
		return d.ErrState
	}
	return nil
}
func (r *Repository) Object(ctx context.Context, tx pgx.Tx, ws uuid.UUID, s d.IdentifierSpec) error {
	if s.ObjectType == "SUBJECT" {
		b, err := r.Binding(ctx, tx, ws, *s.SubjectBindingID, true)
		if err != nil {
			return err
		}
		if b.Status != "BOUND" || b.Provider != s.Provider || b.SubjectID != s.ObjectID {
			return d.ErrState
		}
		return nil
	}
	if s.ObjectType == "PRODUCT_RELEASE" {
		return r.Release(ctx, tx, ws, s.ObjectID, false)
	}
	table := "data_product"
	if s.ObjectType == "DATA_RESOURCE" {
		table = "data_resource"
	}
	var id uuid.UUID
	return Error(tx.QueryRow(ctx, `SELECT id FROM `+table+` WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL FOR SHARE`, s.ObjectID, ws).Scan(&id))
}

const publicationSelect = `SELECT id,workspace_id,provider,identity_binding_id,product_release_id,status,revision,principal_ref,created_by,created_at FROM external_publication`

func scanPublication(row pgx.Row) (d.Publication, error) {
	var p d.Publication
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Provider, &p.IdentityBindingID, &p.ProductReleaseID, &p.Status, &p.Revision, &p.PrincipalRef, &p.CreatedBy, &p.CreatedAt)
	p.Transitions = []d.Transition{}
	return p, Error(err)
}
func (r *Repository) Publication(ctx context.Context, q Querier, ws, id uuid.UUID, lock bool) (d.Publication, error) {
	sql := publicationSelect + ` WHERE workspace_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	return scanPublication(q.QueryRow(ctx, sql, ws, id))
}
func (r *Repository) PublicationHistory(ctx context.Context, ws, id uuid.UUID) (d.Publication, error) {
	// One SQL statement gives projection + history the same MVCC snapshot.
	rows, err := r.Pool.Query(ctx, `SELECT p.id,p.workspace_id,p.provider,p.identity_binding_id,p.product_release_id,p.status,p.revision,p.principal_ref,p.created_by,p.created_at,
 t.id,t.revision,t.from_status,t.to_status,t.evidence_id,t.event_id,t.principal_ref,t.created_by,t.created_at,t.provider_observation_id FROM external_publication p JOIN external_publication_transition t ON t.publication_id=p.id WHERE p.workspace_id=$1 AND p.id=$2 ORDER BY t.revision`, ws, id)
	if err != nil {
		return d.Publication{}, err
	}
	defer rows.Close()
	p := d.Publication{Transitions: []d.Transition{}}
	for rows.Next() {
		var t d.Transition
		err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Provider, &p.IdentityBindingID, &p.ProductReleaseID, &p.Status, &p.Revision, &p.PrincipalRef, &p.CreatedBy, &p.CreatedAt, &t.ID, &t.Revision, &t.FromStatus, &t.ToStatus, &t.EvidenceID, &t.EventID, &t.PrincipalRef, &t.CreatedBy, &t.CreatedAt, &t.ProviderObservationID)
		if err != nil {
			return p, err
		}
		p.Transitions = append(p.Transitions, t)
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	if p.ID == uuid.Nil {
		return p, d.ErrNotFound
	}
	return p, nil
}
func (r *Repository) Publications(ctx context.Context, ws uuid.UUID, limit, offset int) ([]d.Publication, error) {
	rows, err := r.Pool.Query(ctx, publicationSelect+` WHERE workspace_id=$1 ORDER BY created_at,id LIMIT $2 OFFSET $3`, ws, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []d.Publication{}
	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (r *Repository) InsertPublication(ctx context.Context, tx pgx.Tx, p d.Publication) error {
	_, err := tx.Exec(ctx, `INSERT INTO external_publication(id,workspace_id,provider,identity_binding_id,product_release_id,status,revision,principal_ref,created_by,created_at) VALUES($1,$2,$3,$4,$5,'DRAFT',0,$6,$7,$8)`, p.ID, p.WorkspaceID, p.Provider, p.IdentityBindingID, p.ProductReleaseID, p.PrincipalRef, p.CreatedBy, p.CreatedAt)
	return Error(err)
}
func (r *Repository) Submit(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE external_publication SET status='SUBMITTING',revision=1 WHERE id=$1`, id)
	return Error(err)
}
func (r *Repository) Transition(ctx context.Context, tx pgx.Tx, ws, id uuid.UUID, t d.Transition) error {
	_, err := tx.Exec(ctx, `INSERT INTO external_publication_transition(id,publication_id,workspace_id,revision,from_status,to_status,evidence_id,event_id,principal_ref,created_by,created_at,provider_observation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, t.ID, id, ws, t.Revision, t.FromStatus, t.ToStatus, t.EvidenceID, t.EventID, t.PrincipalRef, t.CreatedBy, t.CreatedAt, t.ProviderObservationID)
	return Error(err)
}
func (r *Repository) Invalidate(ctx context.Context, tx pgx.Tx, kind string, ws, id, fact, evidence uuid.UUID, reason, subject string, actor uuid.UUID, at any) error {
	table, column := "external_identity_binding_disposition", "binding_id"
	if kind == "IDENTIFIER" {
		table, column = "external_identifier_disposition", "identifier_id"
	}
	_, err := tx.Exec(ctx, `INSERT INTO `+table+`(id,`+column+`,workspace_id,disposition,reason,evidence_id,principal_ref,created_by,created_at) VALUES($1,$2,$3,'INVALIDATED',$4,$5,$6,$7,$8)`, fact, id, ws, reason, evidence, subject, actor, at)
	return Error(err)
}
func (r *Repository) Replay(ctx context.Context, tx pgx.Tx, ws uuid.UUID, command, key, fp string) (d.Receipt, bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "external/"+ws.String()+"/"+command+"/"+key); err != nil {
		return d.Receipt{}, false, err
	}
	var rec d.Receipt
	var old string
	err := tx.QueryRow(ctx, `SELECT object_id,result_ref,request_fingerprint FROM command_idempotency WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3`, ws, command, key).Scan(&rec.ObjectID, &rec.FactID, &old)
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, err
	}
	if fp != old {
		return rec, false, d.ErrConflict
	}
	// result_ref always points to the command's Evidence, whose source_id is its fact.
	rec.EvidenceID = rec.FactID
	if err := tx.QueryRow(ctx, `SELECT source_id FROM evidence WHERE id=$1 AND workspace_id=$2`, rec.EvidenceID, ws).Scan(&rec.FactID); err != nil {
		return rec, false, Error(err)
	}
	return rec, true, nil
}
func (r *Repository) SaveReceipt(ctx context.Context, tx pgx.Tx, ws uuid.UUID, command, key, fp string, rec d.Receipt) error {
	_, err := tx.Exec(ctx, `INSERT INTO command_idempotency(workspace_id,command_type,idempotency_key,object_id,result_ref,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6)`, ws, command, key, rec.ObjectID, rec.EvidenceID, fp)
	return Error(err)
}
