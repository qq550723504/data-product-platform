package platformmanagement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"time"
)

type PostgresGate struct {
	Pool  *pgxpool.Pool
	Store *bindings.Store
}

func (g PostgresGate) Confirm(ctx context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	if g.Pool == nil || g.Store == nil {
		return principal.Authority{}, principal.ErrAuthorityUnavailable
	}
	tx, err := g.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return principal.Authority{}, principal.ErrAuthorityUnavailable
	}
	defer tx.Rollback(ctx)
	a, err := g.Store.ValidateInTx(ctx, tx, p, principal.CapabilityManagementRead)
	if err != nil {
		return a, err
	}
	if tx.Commit(ctx) != nil {
		return principal.Authority{}, principal.ErrAuthorityUnavailable
	}
	return a, nil
}

// Adapted from accountaudit: cursor binds scope, caller, window and exact position.
type auditCursor struct {
	Workspace, Caller uuid.UUID
	AsOf, Time        time.Time
	ID                uuid.UUID
}

func decodeCursor(raw string, ws, caller uuid.UUID) (auditCursor, error) {
	c := auditCursor{Workspace: ws, Caller: caller, AsOf: time.Now().UTC().Truncate(time.Microsecond)}
	if raw == "" {
		return c, nil
	}
	if len(raw) > 1024 {
		return c, ErrInvalidRequest
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return c, ErrInvalidRequest
	}
	var parsed auditCursor
	if json.Unmarshal(b, &parsed) != nil || parsed.Workspace != ws || parsed.Caller != caller || parsed.ID == uuid.Nil || parsed.AsOf.IsZero() || parsed.Time.IsZero() || !parsed.Time.Before(parsed.AsOf) || parsed.AsOf.After(time.Now().UTC()) {
		return c, ErrInvalidRequest
	}
	canonical, _ := json.Marshal(parsed)
	if base64.RawURLEncoding.EncodeToString(canonical) != raw {
		return c, ErrInvalidRequest
	}
	return parsed, nil
}

type PostgresAudit struct{ Pool *pgxpool.Pool }

func (a PostgresAudit) List(ctx context.Context, ws, caller uuid.UUID, raw string) (AuditPage, error) {
	result := AuditPage{Items: []AuditEvent{}}
	c, err := decodeCursor(raw, ws, caller)
	if err != nil {
		return result, err
	}
	if a.Pool == nil {
		return result, ErrUnavailable
	}
	rows, err := a.Pool.Query(ctx, `SELECT id,actor_id,actor_type,action,object_type,object_id,occurred_at FROM audit_event WHERE workspace_id=$1 AND occurred_at<$2 AND ($3::timestamptz IS NULL OR (occurred_at,id)<($3,$4)) ORDER BY occurred_at DESC,id DESC LIMIT 26`, ws, c.AsOf, nullableTime(c.Time), c.ID)
	if err != nil {
		return result, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var e AuditEvent
		if rows.Scan(&e.ID, &e.ActorID, &e.ActorType, &e.Action, &e.ObjectType, &e.ObjectID, &e.OccurredAt) != nil {
			return AuditPage{}, ErrUnavailable
		}
		result.Items = append(result.Items, e)
	}
	if rows.Err() != nil {
		return AuditPage{}, ErrUnavailable
	}
	if len(result.Items) > 25 {
		result.Items = result.Items[:25]
		last := result.Items[24]
		c.Time = last.OccurredAt
		c.ID = last.ID
		b, _ := json.Marshal(c)
		result.Next = base64.RawURLEncoding.EncodeToString(b)
	}
	return result, nil
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
