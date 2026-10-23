package application

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	remediation "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	workflow "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type Service struct {
	Tx          *transaction.Manager
	Repo        *infrastructure.Repository
	Remediation *remediation.Service
}
type Command struct {
	Principal    principal.RequestPrincipal
	TaskID       uuid.UUID
	Key, TraceID string
}
type Request struct {
	ExpectedRevision  int64                   `json:"expectedRevision,omitempty"`
	BasePlanID        uuid.UUID               `json:"basePlanRevisionId,omitempty"`
	PlanID            uuid.UUID               `json:"planRevisionId,omitempty"`
	ContentHash       string                  `json:"contentHash,omitempty"`
	Phase             string                  `json:"phase,omitempty"`
	SourceVersionID   uuid.UUID               `json:"sourceVersionId,omitempty"`
	Goal              string                  `json:"goal,omitempty"`
	PurposeNote       string                  `json:"purposeNote,omitempty"`
	AcceptanceText    string                  `json:"acceptanceText,omitempty"`
	CaseID            uuid.UUID               `json:"caseId,omitempty"`
	WorkflowVersionID uuid.UUID               `json:"workflowVersionId,omitempty"`
	OutputDatasetID   uuid.UUID               `json:"outputDatasetId,omitempty"`
	TargetPeriod      string                  `json:"targetPeriod,omitempty"`
	Inputs            []workflow.InputBinding `json:"inputs,omitempty"`
	ApprovalID        uuid.UUID               `json:"approvalId,omitempty"`
	Action            string                  `json:"commandKind,omitempty"`
	Reason            string                  `json:"reason"`
}

func (s *Service) prepare(ctx context.Context, tx pgx.Tx, t domain.Task, q Request, ordinal int) (domain.Plan, error) {
	p := domain.Plan{ID: uuid.New(), TaskID: t.ID, WorkspaceID: t.WorkspaceID, Ordinal: ordinal, Phase: q.Phase, SourceVersionID: q.SourceVersionID, Goal: q.Goal, PurposeNote: q.PurposeNote, AcceptanceText: q.AcceptanceText}
	if s.Remediation == nil {
		return p, domain.ErrBinding
	}
	template := s.Remediation.Plan
	p.TemplateRef, p.TemplateVersion, p.TemplateHash = template.Ref, template.Metadata.Version, template.Hash
	p.DetectionRuleRef, p.DetectionRuleHash = template.Spec.DetectionRuleSetRef, template.DetectionHash
	p.WorkflowHash, p.RecheckRuleRef, p.RecheckRuleHash = template.WorkflowHash, template.Spec.RecheckRuleSetRef, template.RecheckHash
	if err := s.source(ctx, tx, t.WorkspaceID, p.SourceVersionID); err != nil {
		return p, err
	}
	switch q.Phase {
	case domain.Diagnosis:
		if q.CaseID != uuid.Nil || q.WorkflowVersionID != uuid.Nil || q.OutputDatasetID != uuid.Nil || q.TargetPeriod != "" || len(q.Inputs) != 0 {
			return p, domain.ErrInvalid
		}
		p.Inputs = []domain.Input{{Name: template.Spec.InputName, VersionID: p.SourceVersionID, Required: true}}
		p.Actions = []domain.Action{{Ordinal: 1, Command: "DETECT", Phase: domain.Diagnosis}}
	case domain.Repair:
		prepared, err := s.Remediation.PrepareAgentRepairTx(ctx, tx, t.WorkspaceID, q.CaseID, q.WorkflowVersionID, q.OutputDatasetID, q.TargetPeriod, q.Inputs)
		if err != nil {
			if remediation.IsPreparationBlocked(err) {
				return p, domain.ErrBinding
			}
			return p, err
		}
		c := prepared.Case
		if c.VersionID != p.SourceVersionID {
			return p, domain.ErrBinding
		}
		p.CaseID = &c.ID
		p.AssessmentID = &c.AssessmentID
		p.FindingID = &c.FindingID
		p.WorkflowVersionID = &q.WorkflowVersionID
		p.OutputDatasetID = &q.OutputDatasetID
		p.TargetPeriod = q.TargetPeriod
		for _, input := range q.Inputs {
			v := domain.Input{Name: input.Name, VersionID: input.DatasetVersionID, Required: true}
			if job, ok := prepared.ResolutionJobs[input.Name]; ok {
				v.ResolutionJobID = &job
			} else {
				v.ReceiptID = &prepared.ReceiptID
			}
			p.Inputs = append(p.Inputs, v)
		}
		p.Actions = []domain.Action{{1, "ASSIGN_SELF", domain.Repair}, {2, "EXECUTE_REPAIR", domain.Repair}, {3, "RECHECK", domain.Repair}}
	default:
		return p, domain.ErrInvalid
	}
	return p, p.Freeze()
}
func (s *Service) source(ctx context.Context, tx pgx.Tx, ws, version uuid.UUID) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=$1 AND d.workspace_id=$2 AND d.dataset_type='RAW' AND v.status='READY')`, version, ws).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return domain.ErrBinding
	}
	return nil
}
func (s *Service) current(ctx context.Context, tx pgx.Tx, p domain.Plan) error {
	if s.Remediation == nil {
		return domain.ErrBinding
	}
	t := s.Remediation.Plan
	if p.TemplateRef != t.Ref || p.TemplateVersion != t.Metadata.Version || p.TemplateHash != t.Hash || p.DetectionRuleHash != t.DetectionHash || p.WorkflowHash != t.WorkflowHash || p.RecheckRuleHash != t.RecheckHash {
		return domain.ErrPlanChanged
	}
	if err := s.source(ctx, tx, p.WorkspaceID, p.SourceVersionID); err != nil {
		return err
	}
	if p.Phase == domain.Repair {
		inputs := make([]workflow.InputBinding, 0, len(p.Inputs))
		for _, i := range p.Inputs {
			inputs = append(inputs, workflow.InputBinding{Name: i.Name, DatasetVersionID: i.VersionID})
		}
		prepared, err := s.Remediation.PrepareAgentRepairTx(ctx, tx, p.WorkspaceID, *p.CaseID, *p.WorkflowVersionID, *p.OutputDatasetID, p.TargetPeriod, inputs)
		if err != nil {
			if remediation.IsPreparationBlocked(err) {
				return domain.ErrBinding
			}
			return err
		}
		for _, i := range p.Inputs {
			if i.ReceiptID != nil && *i.ReceiptID != prepared.ReceiptID {
				return domain.ErrPlanChanged
			}
			if i.ResolutionJobID != nil && *i.ResolutionJobID != prepared.ResolutionJobs[i.Name] {
				return domain.ErrPlanChanged
			}
		}
	}
	return nil
}

// Handle records finite planning facts only. It never calls a business engine.
func (s *Service) Handle(ctx context.Context, c Command, kind string, q Request) (domain.Receipt, error) {
	var result domain.Receipt
	p := c.Principal
	if p.ActorID == uuid.Nil || p.WorkspaceID == uuid.Nil || p.Subject == "" || !p.HasCapability(principal.CapabilityHumanDecision) {
		return result, domain.ErrDenied
	}
	if !domain.Text(c.Key, 1, 128) || !domain.Text(q.Reason, 1, 1000) {
		return result, domain.ErrInvalid
	}
	if kind != "CREATE" && c.TaskID == uuid.Nil {
		return result, domain.ErrInvalid
	}
	hash, _, err := domain.Hash(map[string]any{"actorId": p.ActorID, "subject": p.Subject, "command": kind, "taskId": c.TaskID, "request": q})
	if err != nil {
		return result, err
	}
	err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var task domain.Task
		if kind == "CREATE" {
			// CREATE has no task row yet; serialize its scoped idempotency key.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "agent-create/"+p.WorkspaceID.String()+"/"+c.Key); err != nil {
				return err
			}
		} else {
			var err error
			task, err = s.Repo.TaskTx(ctx, tx, p.WorkspaceID, c.TaskID, true)
			if err != nil {
				return err
			}
			if task.CreatedBy != p.ActorID || task.Subject != p.Subject {
				return domain.ErrDenied
			}
		}
		old, found, err := s.Repo.ReceiptTx(ctx, tx, p.WorkspaceID, c.TaskID, kind, c.Key, hash)
		if err != nil {
			return err
		}
		if found {
			result = old
			return nil
		}
		result = domain.Receipt{ID: uuid.New()}
		event := ""
		if kind == "CREATE" {
			if q.Phase != domain.Diagnosis {
				return domain.ErrInvalid
			}
			task = domain.Task{ID: uuid.New(), WorkspaceID: p.WorkspaceID, CreatedBy: p.ActorID, Subject: p.Subject, Revision: 1, ControlState: "ACTIVE"}
			plan, err := s.prepare(ctx, tx, task, q, 1)
			if err != nil {
				return err
			}
			task.CurrentPlanRevisionID = plan.ID
			if _, err = tx.Exec(ctx, `INSERT INTO agent_task(id,workspace_id,kind,created_by,subject,current_plan_revision_id,revision,control_state) VALUES($1,$2,'QUALITY_DIAGNOSIS',$3,$4,$5,1,'ACTIVE')`, task.ID, p.WorkspaceID, p.ActorID, p.Subject, plan.ID); err != nil {
				return err
			}
			if err = s.Repo.InsertPlan(ctx, tx, plan); err != nil {
				return err
			}
			event = "AgentTaskCreated"
		} else {
			if task.Revision != q.ExpectedRevision {
				return domain.ErrStale
			}
			if task.ControlState == "STOPPED" {
				return domain.ErrStopped
			}
			plan, err := s.Repo.PlanTx(ctx, tx, task.ID, task.CurrentPlanRevisionID)
			if err != nil {
				return err
			}
			switch kind {
			case "REVISE":
				if err = domain.CanMutate(task, q.ExpectedRevision); err != nil {
					return err
				}
				if q.BasePlanID != plan.ID {
					return domain.ErrPlanChanged
				}
				pending, err := s.Repo.PendingTx(ctx, tx, task.ID)
				if err != nil {
					return err
				}
				if pending {
					return domain.ErrUnresolved
				}
				next, err := s.prepare(ctx, tx, task, q, plan.Ordinal+1)
				if err != nil {
					return err
				}
				if err = s.Repo.InsertPlan(ctx, tx, next); err != nil {
					return err
				}
				task.CurrentPlanRevisionID = next.ID
				event = "AgentPlanRevised"
			case "APPROVE", "REQUEST_ACTION":
				if err = domain.CanMutate(task, q.ExpectedRevision); err != nil {
					return err
				}
				if q.PlanID != plan.ID || q.ContentHash != plan.ContentHash || q.Phase != plan.Phase {
					return domain.ErrPlanChanged
				}
				if err = s.current(ctx, tx, plan); err != nil {
					return err
				}
				if kind == "APPROVE" {
					pending, err := s.Repo.PendingTx(ctx, tx, task.ID)
					if err != nil {
						return err
					}
					if pending {
						return domain.ErrUnresolved
					}
					var exists bool
					if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_approval WHERE plan_revision_id=$1 AND phase=$2)`, plan.ID, plan.Phase).Scan(&exists); err != nil {
						return err
					}
					if exists {
						return domain.ErrApproval
					}
					id := uuid.New()
					scope, _, err := domain.Hash(map[string]any{"planId": plan.ID, "hash": plan.ContentHash, "phase": plan.Phase, "actorId": p.ActorID, "subject": p.Subject})
					if err != nil {
						return err
					}
					if _, err = tx.Exec(ctx, `INSERT INTO agent_approval(id,task_id,plan_revision_id,content_hash,phase,actor_id,subject,scope_hash,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, task.ID, plan.ID, plan.ContentHash, plan.Phase, p.ActorID, p.Subject, scope, q.Reason); err != nil {
						return err
					}
					result.ApprovalID = &id
					event = "AgentPlanApproved"
				} else {
					var valid bool
					if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_approval WHERE id=$1 AND task_id=$2 AND plan_revision_id=$3 AND content_hash=$4 AND actor_id=$5 AND subject=$6 AND phase=$7)`, q.ApprovalID, task.ID, plan.ID, plan.ContentHash, p.ActorID, p.Subject, plan.Phase).Scan(&valid); err != nil {
						return err
					}
					if !valid {
						return domain.ErrApproval
					}
					// Later slots need actual owner receipts from Q3/Q4, which Q2 cannot invent.
					if len(plan.Actions) == 0 || q.Action != plan.Actions[0].Command {
						return domain.ErrOrder
					}
					var used bool
					if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_action_request WHERE plan_revision_id=$1 AND ordinal=1)`, plan.ID).Scan(&used); err != nil {
						return err
					}
					if used {
						return domain.ErrSlotUsed
					}
					id := uuid.New()
					if _, err = tx.Exec(ctx, `INSERT INTO agent_action_request(id,task_id,workspace_id,plan_revision_id,approval_id,ordinal,command_kind,request_key,fingerprint,business_request_key,dispatch_state,reason) VALUES($1,$2,$3,$4,$5,1,$6,$7,$8,$9,'REQUESTED',$10)`, id, task.ID, p.WorkspaceID, plan.ID, q.ApprovalID, q.Action, c.Key, hash, "agent-"+id.String(), q.Reason); err != nil {
						return err
					}
					result.ActionID = &id
					result.ApprovalID = &q.ApprovalID
					event = "AgentActionRequested"
				}
			case "PAUSE":
				if task.ControlState != "ACTIVE" {
					return domain.ErrPaused
				}
				task.ControlState = "PAUSED"
				event = "AgentTaskPaused"
			case "RESUME":
				if task.ControlState != "PAUSED" {
					return domain.ErrInvalid
				}
				if err = s.current(ctx, tx, plan); err != nil {
					return err
				}
				task.ControlState = "ACTIVE"
				event = "AgentTaskResumed"
			case "STOP":
				task.ControlState = "STOPPED"
				event = "AgentTaskStopped"
			default:
				return domain.ErrInvalid
			}
			task.Revision++
			if _, err = tx.Exec(ctx, `UPDATE agent_task SET current_plan_revision_id=$2,revision=$3,control_state=$4 WHERE id=$1`, task.ID, task.CurrentPlanRevisionID, task.Revision, task.ControlState); err != nil {
				return err
			}
			if kind == "STOP" {
				if _, err = tx.Exec(ctx, `INSERT INTO agent_action_disposition(id,task_id,action_id,disposition,actor_id,subject,reason) SELECT gen_random_uuid(),$1,a.id,'CANCELLED',$2,$3,$4 FROM agent_action_request a WHERE a.task_id=$1 AND NOT EXISTS(SELECT 1 FROM agent_action_disposition d WHERE d.action_id=a.id)`, task.ID, p.ActorID, p.Subject, q.Reason); err != nil {
					return err
				}
			}
		}
		result.TaskID, result.PlanID, result.TaskRevision = task.ID, task.CurrentPlanRevisionID, task.Revision
		if _, err = tx.Exec(ctx, `INSERT INTO agent_command_receipt(id,workspace_id,command_kind,target_id,request_key,fingerprint,actor_id,subject,task_id,plan_revision_id,approval_id,action_id,task_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, result.ID, p.WorkspaceID, kind, c.TaskID, c.Key, hash, p.ActorID, p.Subject, result.TaskID, result.PlanID, result.ApprovalID, result.ActionID, result.TaskRevision); err != nil {
			return err
		}
		payload := map[string]any{"receiptId": result.ID, "taskId": task.ID, "planRevisionId": result.PlanID, "taskRevision": task.Revision, "approvalId": result.ApprovalID, "actionId": result.ActionID, "workspaceId": p.WorkspaceID, "actorId": p.ActorID, "subject": p.Subject, "dispatchEnabled": false}
		e, err := outbox.NewEvent("AGENT_TASK", task.ID, event, payload)
		if err != nil {
			return err
		}
		if err = outbox.Append(ctx, tx, e); err != nil {
			return err
		}
		if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &p.WorkspaceID, ActorType: "USER", ActorID: &p.ActorID, Action: event, ObjectType: "AGENT_TASK", ObjectID: task.ID, AfterState: payload, Reason: q.Reason, TraceID: c.TraceID}); err != nil {
			return err
		}
		_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: p.WorkspaceID, EvidenceType: "AGENT_TASK", Title: event, SourceType: "AGENT_TASK", SourceID: &task.ID, Metadata: payload, CreatedBy: &p.ActorID}, evidence.Relation{ObjectType: "AGENT_TASK", ObjectID: task.ID, RelationType: "SUPPORTS"})
		return err
	})
	return result, err
}

func IsConflict(err error) bool {
	for _, e := range []error{domain.ErrConflict, domain.ErrStale, domain.ErrPlanChanged, domain.ErrUnresolved, domain.ErrStopped, domain.ErrPaused, domain.ErrApproval, domain.ErrSlotUsed, domain.ErrOrder, domain.ErrBinding} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
