package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	me "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

var ErrSchemaDenied = errors.New("source schema observation authority denied")

func (s *SchemaService) currentBinding(ctx context.Context, tx pgx.Tx, req d.SchemaRequest) error {
	var current bool
	// Keep this exact binding current until the observation transaction commits.
	// Refresh/demotion UPDATEs take the same tuple lock, including direct SQL.
	err := tx.QueryRow(ctx, `SELECT true FROM resource_binding WHERE id=$1 AND resource_id=$2 AND external_id=$3 AND external_fqn=$4 AND provider=$5 AND entity_type='TABLE' AND is_primary FOR UPDATE`, req.BindingID, req.Plan.ResourceID, req.ExternalID, req.Plan.ExternalFQN, s.Service.provider).Scan(&current)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSchemaDenied
		}
		return err
	}
	if !current {
		return ErrSchemaDenied
	}
	return nil
}

type SchemaService struct {
	Service  *Service
	Observer me.SourceSchemaObserver
	Rights   schemaEntitlementChecker
}

type schemaEntitlementChecker interface {
	CheckCurrentEntitlementTx(context.Context, pgx.Tx, rd.EntitlementRequest) (rd.EntitlementDecision, error)
}

func (s *SchemaService) authorize(ctx context.Context, tx pgx.Tx, p d.SchemaPlan) error {
	current, err := s.Observer.Plan(ctx)
	if err != nil {
		return err
	}
	if current.Validate() != nil || d.Hash(current) != d.Hash(p) {
		return ErrSchemaDenied
	}
	if _, err := deliveryfence.Lock(ctx, tx, p.WorkspaceID); err != nil {
		return err
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT workspace_id=$2 AND lifecycle_status NOT IN ('BLOCKED','ARCHIVED') AND deleted_at IS NULL FROM data_resource WHERE id=$1 FOR UPDATE`, p.ResourceID, p.WorkspaceID).Scan(&allowed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSchemaDenied
		}
		return err
	}
	if !allowed {
		return ErrSchemaDenied
	}
	decision, err := s.Rights.CheckCurrentEntitlementTx(ctx, tx, rd.EntitlementRequest{WorkspaceID: p.WorkspaceID, AuthorizationID: p.AuthorizationID, DataResourceID: p.ResourceID, ConsumerRef: p.ConsumerRef, Purpose: p.Purpose, Action: "READ", Scope: rd.NormalizedScope{Type: "ALL_RESOURCE", Ref: p.ResourceID.String()}, AsOf: time.Now().UTC(), Path: rd.EntitlementDownstream})
	if err != nil {
		return err
	}
	if decision.Decision != "ALLOWED" {
		return ErrSchemaDenied
	}
	return nil
}
func (s *SchemaService) fact(ctx context.Context, tx pgx.Tx, req d.SchemaRequest, event string, payload any) error {
	e, err := outbox.NewEvent("SOURCE_SCHEMA_REQUEST", req.ID, event, payload)
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, e); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{WorkspaceID: &req.Plan.WorkspaceID, ActorType: "USER", ActorID: &req.Plan.ActorID, Action: event, ObjectType: "SOURCE_SCHEMA_REQUEST", ObjectID: req.ID, AfterState: payload}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: req.Plan.WorkspaceID, EvidenceType: "SOURCE_SCHEMA_OBSERVATION", Title: event, SourceType: "SOURCE_SCHEMA_REQUEST", SourceID: &req.ID, CreatedBy: &req.Plan.ActorID, Metadata: map[string]any{"event": event, "fact": payload}}, evidence.Relation{ObjectType: "SOURCE_SCHEMA_REQUEST", ObjectID: req.ID, RelationType: "SUPPORTS"})
	return err
}
func (s *SchemaService) Request(ctx context.Context, resource, ws, actor uuid.UUID, key string) (d.SchemaRequest, error) {
	var req d.SchemaRequest
	if s == nil || s.Observer == nil || s.Rights == nil || key == "" || len(key) > 128 {
		return req, ErrSchemaDenied
	}
	p, err := s.Observer.Plan(ctx)
	if err != nil {
		return req, err
	}
	if p.ResourceID != resource || p.WorkspaceID != ws || p.ActorID != actor {
		return req, ErrSchemaDenied
	}
	req = d.SchemaRequest{ID: uuid.New(), Plan: p, PlanHash: d.Hash(p), Key: key}
	err = s.Service.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, p); err != nil {
			return err
		}
		// Bind the exact provider identity under the same resource lock. No body FQN.
		if err := tx.QueryRow(ctx, `SELECT id,external_id FROM resource_binding WHERE resource_id=$1 AND external_fqn=$2 AND entity_type='TABLE' AND provider=$3 AND is_primary`, resource, p.ExternalFQN, s.Service.provider).Scan(&req.BindingID, &req.ExternalID); err != nil {
			return ErrSchemaDenied
		}
		var fresh bool
		var err error
		req, fresh, err = s.Service.repo.PrepareSchema(ctx, tx, req)
		if err != nil || !fresh {
			return err
		}
		return s.fact(ctx, tx, req, "SourceSchemaObservationRequested", map[string]any{"requestId": req.ID, "resourceId": resource, "ruleRef": p.RuleRef, "planHash": req.PlanHash, "responsibleActorId": actor})
	})
	return req, err
}
func (s *SchemaService) Process(ctx context.Context, id uuid.UUID) error {
	req, err := s.Service.repo.SchemaRequest(ctx, id)
	if err != nil {
		return err
	}
	// One source scanning owner; unrelated sources and Core facts are independent.
	return s.Service.tx.WithAdvisoryLock(ctx, "source-schema:"+req.Plan.ResourceID.String(), func(ctx context.Context) error {
		pending, err := s.Service.repo.PendingSchemaAttempts(ctx, req.Plan.ResourceID)
		if err != nil {
			return err
		}
		for _, priorAttempt := range pending {
			priorRequest := priorAttempt.RequestID
			if err := s.Observer.Stop(ctx, priorAttempt.ID, priorAttempt.Runtime); err != nil {
				return err
			}
			prior, err := s.Service.repo.SchemaRequest(ctx, priorRequest)
			if err != nil {
				return err
			}
			if err := s.Service.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				return s.record(ctx, tx, prior, d.SchemaObservation{ID: uuid.New(), RequestID: priorRequest, AttemptID: &priorAttempt.ID, Status: "INTERRUPTED", Reason: "PROCESS_INTERRUPTED"})
			}); err != nil {
				return err
			}
		}
		var done, denied bool
		attempt := uuid.New()
		var runtime json.RawMessage
		err = s.Service.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			done, err = s.Service.repo.SchemaTerminal(ctx, tx, id)
			if err != nil || done {
				return err
			}
			if err := s.authorize(ctx, tx, req.Plan); err != nil {
				if !errors.Is(err, ErrSchemaDenied) {
					return err
				}
				denied = true
				return s.record(ctx, tx, req, d.SchemaObservation{ID: uuid.New(), RequestID: id, Status: "DENIED", Reason: "CURRENT_AUTHORITY_DENIED"})
			}
			if err := s.currentBinding(ctx, tx, req); err != nil {
				if !errors.Is(err, ErrSchemaDenied) {
					return err
				}
				denied = true
				return s.record(ctx, tx, req, d.SchemaObservation{ID: uuid.New(), RequestID: id, Status: "DENIED", Reason: "SOURCE_BINDING_CHANGED"})
			}
			runtime, err = s.Observer.Prepare(ctx, attempt, req.Plan)
			if err != nil {
				return err
			}
			if err := s.Service.repo.BeginSchemaAttempt(ctx, tx, req, attempt, runtime); err != nil {
				return err
			}
			hash, err := d.RuntimeHash(runtime)
			if err != nil {
				return err
			}
			return s.fact(ctx, tx, req, "SourceSchemaScanStarted", map[string]any{"attemptId": attempt, "ruleRef": req.Plan.RuleRef, "runtimeHash": hash})
		})
		if err != nil || done || denied {
			return err
		}
		// Start identity is durable before the native process invocation. Physical
		// invocation cost is durable independently from a successful observation.
		result, scanErr := s.Observer.Scan(ctx, attempt, runtime, func() error {
			// Includes failed/hung startup before any marker; same attempt dedupes.
			return s.ChargeScan(context.WithoutCancel(ctx), attempt)
		}, func() error {
			// RUN still awaits fresh source authority after the bound marker.
			return s.Service.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				if err := s.authorize(ctx, tx, req.Plan); err != nil {
					return err
				}
				return s.currentBinding(ctx, tx, req)
			})
		})
		// Successful Scan proves the native process has exited. Cleanup must not
		// discard that completed result by rereading a temporarily missing profile.
		if scanErr != nil {
			if err := s.Observer.Stop(context.WithoutCancel(ctx), attempt, runtime); err != nil {
				return err
			}
		}
		o := d.SchemaObservation{ID: uuid.New(), RequestID: id, AttemptID: &attempt, Status: "UNAVAILABLE", Reason: "NATIVE_SOURCE_SCAN_UNAVAILABLE"}
		if scanErr == nil && result.ExternalID == req.ExternalID && result.ExternalFQN == req.Plan.ExternalFQN && result.ConnectorHash == req.Plan.ConnectorHash && d.ValidateColumns(result.Columns) == nil {
			o.Columns = result.Columns
			o.SchemaHash = d.Hash(result.Columns)
			o.StartedAt = &result.StartedAt
			o.CompletedAt = &result.CompletedAt
			o.Status = "MATCH"
			o.Reason = "BASELINE_MATCH"
			if o.SchemaHash != d.Hash(req.Plan.Columns) {
				o.Status = "DRIFT"
				o.Reason = "SOURCE_SCHEMA_CHANGED"
			}
		}
		if err := s.Service.tx.Do(context.WithoutCancel(ctx), func(ctx context.Context, tx pgx.Tx) error {
			gateErr := s.authorize(ctx, tx, req.Plan)
			deniedReason := "CURRENT_AUTHORITY_DENIED"
			if gateErr == nil {
				gateErr = s.currentBinding(ctx, tx, req)
				deniedReason = "SOURCE_BINDING_CHANGED"
			}
			if err := gateErr; err != nil {
				if !errors.Is(err, ErrSchemaDenied) {
					return err
				}
				o.Status = "DENIED"
				o.Reason = deniedReason
				o.Columns = nil
				o.SchemaHash = ""
			}
			return s.record(ctx, tx, req, o)
		}); err != nil {
			return err
		}
		if o.Status == "UNAVAILABLE" {
			return errors.New("source schema scan unavailable; durable observation retained")
		}
		return nil
	})
}
func (s *SchemaService) record(ctx context.Context, tx pgx.Tx, req d.SchemaRequest, o d.SchemaObservation) error {
	if err := s.Service.repo.AppendSchemaObservation(ctx, tx, o); err != nil {
		return err
	}
	return s.fact(ctx, tx, req, "SourceSchemaObserved", map[string]any{"observationId": o.ID, "attemptId": o.AttemptID, "status": o.Status, "reason": o.Reason, "schemaHash": o.SchemaHash, "resourceId": req.Plan.ResourceID})
}
func (s *SchemaService) Handle(ctx context.Context, resource, ws, actor, observation uuid.UUID, key, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 1000 || key == "" || len(key) > 128 {
		return ErrSchemaDenied
	}
	return s.Service.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := s.Observer.Plan(ctx)
		if err != nil {
			return err
		}
		if p.ResourceID != resource || p.WorkspaceID != ws || p.ActorID != actor {
			return ErrSchemaDenied
		}
		if err := s.authorize(ctx, tx, p); err != nil {
			return err
		}
		fresh, err := s.Service.repo.HandleSchema(ctx, tx, resource, ws, actor, observation, key, reason)
		if err != nil || !fresh {
			return err
		}
		var reqID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT request_id FROM source_schema_observation WHERE id=$1`, observation).Scan(&reqID); err != nil {
			return err
		}
		return s.fact(ctx, tx, d.SchemaRequest{ID: reqID, Plan: p}, "SourceSchemaHandlingRecorded", map[string]any{"observationId": observation, "reason": reason, "progress": "INVESTIGATING", "responsibleActorId": actor})
	})
}

// ChargeScan is called only after the native process actually starts.
func (s *SchemaService) ChargeScan(ctx context.Context, attempt uuid.UUID) error {
	return s.Service.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var ws uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id FROM source_schema_attempt WHERE id=$1 FOR UPDATE`, attempt).Scan(&ws); err != nil {
			return err
		}
		var charged bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cost_allocation WHERE source_schema_attempt_id=$1)`, attempt).Scan(&charged); err != nil {
			return err
		}
		if charged {
			return nil
		}
		id := uuid.New()
		if err := cost.Append(ctx, tx, cost.Event{ID: id, WorkspaceID: ws, ActivityID: attempt, CostType: "SOURCE_SCHEMA_SCAN_INVOCATION", Quantity: 1, Unit: "invocation", PricingMode: "ACTUAL"}); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,source_schema_attempt_id) VALUES($1,$2,$3)`, uuid.New(), id, attempt)
		return err
	})
}
