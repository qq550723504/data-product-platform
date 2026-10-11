package infrastructure

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

var ErrAlreadyReleased = errors.New("lifecycle occupancy already released")

// The caller holds the workspace fence. Discover first, then lock all Dataset
// parents, all Versions, and finally physical copies in stable UUID order.
func (r *PostgresRepository) LockFixtureCopies(ctx context.Context, tx pgx.Tx, workspace uuid.UUID, ids []uuid.UUID) ([]domain.Copy, error) {
	copies, err := r.LoadFixtureCopies(ctx, tx, workspace, ids)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT v.dataset_id FROM lifecycle_fixture_copy c
		JOIN dataset_version v ON v.id=c.dataset_version_id JOIN dataset d ON d.id=v.dataset_id
		WHERE c.workspace_id=$1 AND d.workspace_id=$1 AND c.id=ANY($2) ORDER BY v.dataset_id`, workspace, ids)
	if err != nil {
		return nil, err
	}
	var datasets []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		datasets = append(datasets, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(datasets) == 0 {
		return nil, ErrScopeUnavailable
	}
	for _, id := range datasets {
		var found uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM dataset WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspace, id).Scan(&found); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrScopeUnavailable
			}
			return nil, err
		}
	}
	versions := make(map[uuid.UUID]bool)
	for _, c := range copies {
		versions[c.DatasetVersionID] = true
	}
	var versionIDs []uuid.UUID
	for id := range versions {
		versionIDs = append(versionIDs, id)
	}
	sort.Slice(versionIDs, func(i, j int) bool { return versionIDs[i].String() < versionIDs[j].String() })
	for _, id := range versionIDs {
		var found uuid.UUID
		// Re-check the parent identity after locking it; a mutable pre-ready
		// version must not move to an unheld Dataset between discovery and lock.
		if err := tx.QueryRow(ctx, `SELECT v.id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id
			WHERE d.workspace_id=$1 AND v.id=$2 AND v.dataset_id=ANY($3) FOR UPDATE OF v`, workspace, id, datasets).Scan(&found); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrScopeUnavailable
			}
			return nil, err
		}
	}
	for _, c := range copies {
		var found uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspace, c.ID).Scan(&found); err != nil {
			return nil, err
		}
	}
	return copies, nil
}

func (r *PostgresRepository) FixtureCopyIDsForVersion(ctx context.Context, tx pgx.Tx, w, version uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE workspace_id=$1 AND dataset_version_id=$2 ORDER BY id`, w, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrScopeUnavailable
	}
	return ids, nil
}

func (r *PostgresRepository) IsUseRestricted(ctx context.Context, tx pgx.Tx, w, version uuid.UUID) (bool, error) {
	var restricted bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_content_use_restriction WHERE workspace_id=$1 AND dataset_version_id=$2)`, w, version).Scan(&restricted)
	return restricted, err
}

func (r *PostgresRepository) HoldCopy(ctx context.Context, tx pgx.Tx, w, holdID uuid.UUID) (uuid.UUID, time.Time, error) {
	var copyID uuid.UUID
	var occurred time.Time
	err := tx.QueryRow(ctx, `SELECT copy_id,occurred_at FROM lifecycle_content_hold WHERE workspace_id=$1 AND id=$2`, w, holdID).Scan(&copyID, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrScopeUnavailable
	}
	return copyID, occurred, err
}

func (r *PostgresRepository) HoldReleased(ctx context.Context, tx pgx.Tx, w, holdID uuid.UUID) (bool, error) {
	var released bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_content_hold_release WHERE workspace_id=$1 AND hold_id=$2)`, w, holdID).Scan(&released)
	return released, err
}

func (r *PostgresRepository) InsertReaderPin(ctx context.Context, tx pgx.Tx, p domain.ReaderPin) error {
	if err := p.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO lifecycle_reader_pin(id,workspace_id,copy_id,reader_id,kind,actor_id,principal_ref,
		generation,gate_revision,started_at,window_mode,use_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		p.ID, p.WorkspaceID, p.CopyID, p.ReaderID, p.Kind, p.ActorID, p.PrincipalRef, p.Generation, p.GateRevision, p.StartedAt, p.WindowMode, p.UseUntil)
	return err
}

func (r *PostgresRepository) GetReaderPin(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) (domain.ReaderPin, error) {
	var p domain.ReaderPin
	err := tx.QueryRow(ctx, `SELECT id,workspace_id,copy_id,reader_id,kind,actor_id,principal_ref,generation,gate_revision,started_at,window_mode,use_until
		FROM lifecycle_reader_pin WHERE workspace_id=$1 AND id=$2`, w, id).Scan(&p.ID, &p.WorkspaceID, &p.CopyID, &p.ReaderID, &p.Kind, &p.ActorID, &p.PrincipalRef, &p.Generation, &p.GateRevision, &p.StartedAt, &p.WindowMode, &p.UseUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrScopeUnavailable
	}
	return p, err
}

func (r *PostgresRepository) ReaderTerminated(ctx context.Context, tx pgx.Tx, w, pinID uuid.UUID) (bool, error) {
	var terminated bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_reader_termination WHERE workspace_id=$1 AND pin_id=$2)`, w, pinID).Scan(&terminated)
	return terminated, err
}

// Only current fixture occupancy is reported. Other material/shared/capability
// dependencies are outside this resolver and must remain UNKNOWN at dispatch.
func (r *PostgresRepository) CopyOccupancy(ctx context.Context, tx pgx.Tx, c domain.Copy) (domain.Occupancy, error) {
	o := domain.Occupancy{CopyID: c.ID, UseRestricted: domain.Denied, HoldFree: domain.Satisfied, ReadersDrained: domain.Satisfied}
	restricted, err := r.IsUseRestricted(ctx, tx, c.WorkspaceID, c.DatasetVersionID)
	if err != nil {
		return o, err
	}
	if restricted {
		o.UseRestricted = domain.Satisfied
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_content_hold h WHERE h.workspace_id=$1 AND h.copy_id=$2
		AND NOT EXISTS(SELECT 1 FROM lifecycle_content_hold_release r WHERE r.hold_id=h.id)`, c.WorkspaceID, c.ID).Scan(&o.ActiveHolds)
	if err != nil {
		return o, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reader_pin p WHERE p.workspace_id=$1 AND p.copy_id=$2
		AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination r WHERE r.pin_id=p.id)`, c.WorkspaceID, c.ID).Scan(&o.ActiveReaders)
	if o.ActiveHolds > 0 {
		o.HoldFree = domain.Denied
	}
	if o.ActiveReaders > 0 {
		o.ReadersDrained = domain.Denied
	}
	if err != nil {
		return o, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_disposition_dispatch d WHERE d.workspace_id=$1 AND d.copy_id=$2
		AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id)`, c.WorkspaceID, c.ID).Scan(&o.UnresolvedDispatches)
	return o, err
}
