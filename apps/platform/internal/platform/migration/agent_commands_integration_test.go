package migration_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	remediation "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	"strings"
	"sync"
	"testing"
)

func TestAgentCommandsRealPostgresReplayRevisionControlAndAtomicProof(t *testing.T) {
	pool := scratchDatabase(t, 73)
	ctx := context.Background()
	ws, actor, ds, source := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,'AGENT-COMMAND','Agent test','RAW')`, ds, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_algorithm,checksum_value,ready_at) VALUES($1,$2,1,'READY','s3://agent-test/source.csv','SHA256',$3,now())`, source, ds, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	template := remediation.Plan{Ref: "test/template", Hash: strings.Repeat("a", 64), DetectionHash: strings.Repeat("b", 64), WorkflowHash: strings.Repeat("c", 64), RecheckHash: strings.Repeat("d", 64)}
	template.Metadata.Version = "1"
	template.Spec.InputName = "source"
	template.Spec.DetectionRuleSetRef = "test/detect"
	template.Spec.RecheckRuleSetRef = "test/recheck"
	svc := application.Service{Tx: transaction.NewManager(pool), Repo: &infrastructure.Repository{Pool: pool}, Remediation: &remediation.Service{Plan: template}}
	identity := principal.RequestPrincipal{WorkspaceID: ws, ActorID: actor, Subject: "test/operator", Capabilities: map[string]struct{}{principal.CapabilityHumanDecision: {}}}
	c := application.Command{Principal: identity, Key: "create"}
	q := application.Request{Phase: domain.Diagnosis, SourceVersionID: source, Goal: "诊断质量", AcceptanceText: "核实发现", Reason: "测试委托"}
	results := make([]domain.Receipt, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; results[i], errs[i] = svc.Handle(ctx, c, "CREATE", q) }(i)
	}
	close(start)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || results[0] != results[1] {
		t.Fatalf("concurrent replay: %+v %+v", results, errs)
	}
	created := results[0]
	changed := q
	changed.Goal = "different"
	if _, err = svc.Handle(ctx, c, "CREATE", changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed payload: %v", err)
	}
	c.TaskID = created.TaskID
	c.Key = "approve"
	read := func() domain.Detail {
		t.Helper()
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		d, e := svc.Repo.DetailTx(ctx, tx, ws, c.TaskID, uuid.Nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	d := read()
	approval := application.Request{ExpectedRevision: 1, PlanID: d.Plan.ID, ContentHash: d.Plan.ContentHash, Phase: d.Plan.Phase, Reason: "确认诊断范围"}
	approved, err := svc.Handle(ctx, c, "APPROVE", approval)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.Handle(ctx, c, "APPROVE", approval)
	if err != nil || replay.ID != approved.ID {
		t.Fatalf("stale revision replay failed: %v", err)
	}
	c.Key = "revise"
	revision := q
	revision.ExpectedRevision = 2
	revision.BasePlanID = d.Plan.ID
	revision.Goal = "修订目标"
	revised, err := svc.Handle(ctx, c, "REVISE", revision)
	if err != nil {
		t.Fatal(err)
	}
	c.Key = "old-approval"
	action := approval
	action.ExpectedRevision = 3
	action.ApprovalID = *approved.ApprovalID
	action.Action = "DETECT"
	if _, err = svc.Handle(ctx, c, "REQUEST_ACTION", action); !errors.Is(err, domain.ErrPlanChanged) {
		t.Fatalf("old approval accepted: %v", err)
	}
	d = read()
	c.Key = "new-approval"
	approval.ExpectedRevision = 3
	approval.PlanID = revised.PlanID
	approval.ContentHash = d.Plan.ContentHash
	approved, err = svc.Handle(ctx, c, "APPROVE", approval)
	if err != nil {
		t.Fatal(err)
	}
	c.Key = "intent"
	action = approval
	action.ExpectedRevision = 4
	action.ApprovalID = *approved.ApprovalID
	action.Action = "DETECT"
	accepted, err := svc.Handle(ctx, c, "REQUEST_ACTION", action)
	if err != nil {
		t.Fatal(err)
	}
	replay, err = svc.Handle(ctx, c, "REQUEST_ACTION", action)
	if err != nil || replay.ID != accepted.ID {
		t.Fatal("intent replay failed", err)
	}
	c.Key = "duplicate-slot"
	action.ExpectedRevision = 5
	if _, err = svc.Handle(ctx, c, "REQUEST_ACTION", action); !errors.Is(err, domain.ErrSlotUsed) {
		t.Fatalf("slot repeated: %v", err)
	}
	c.Key = "pending-revise"
	revision.ExpectedRevision = 5
	revision.BasePlanID = d.Plan.ID
	if _, err = svc.Handle(ctx, c, "REVISE", revision); !errors.Is(err, domain.ErrUnresolved) {
		t.Fatalf("unresolved action hidden: %v", err)
	}
	foreign := c
	foreign.Principal.ActorID = uuid.New()
	if _, err = svc.Handle(ctx, foreign, "STOP", application.Request{ExpectedRevision: 5, Reason: "stop"}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("foreign actor accepted", err)
	}
	foreign.Principal = identity
	foreign.Principal.WorkspaceID = uuid.New()
	if _, err = svc.Handle(ctx, foreign, "STOP", application.Request{ExpectedRevision: 5, Reason: "stop"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("foreign workspace accepted", err)
	}
	for _, step := range []struct {
		kind     string
		revision int64
	}{{"PAUSE", 5}, {"RESUME", 6}, {"STOP", 7}} {
		c.Key = step.kind
		if _, err = svc.Handle(ctx, c, step.kind, application.Request{ExpectedRevision: step.revision, Reason: "控制测试"}); err != nil {
			t.Fatal(step.kind, err)
		}
	}
	d = read()
	if d.Task.ControlState != "STOPPED" || d.Task.Revision != 8 || len(d.Actions) != 1 || d.Actions[0].State != "CANCELLED" {
		t.Fatalf("stop projection: %+v", d)
	}
	var receipts, audits, evidences, events, intents int
	for _, v := range []struct {
		sql string
		dst *int
	}{{`SELECT count(*) FROM agent_command_receipt WHERE task_id=$1`, &receipts}, {`SELECT count(*) FROM audit_event WHERE object_id=$1`, &audits}, {`SELECT count(*) FROM evidence WHERE source_id=$1`, &evidences}, {`SELECT count(*) FROM outbox_event WHERE aggregate_id=$1`, &events}, {`SELECT count(*) FROM agent_action_request WHERE task_id=$1`, &intents}} {
		if err = pool.QueryRow(ctx, v.sql, created.TaskID).Scan(v.dst); err != nil {
			t.Fatal(err)
		}
	}
	if receipts != 8 || audits != 8 || evidences != 8 || events != 8 || intents != 1 {
		t.Fatalf("proofs %d %d %d %d intents %d", receipts, audits, evidences, events, intents)
	}
	var obligation []string
	if err = pool.QueryRow(ctx, `SELECT required_handlers FROM outbox_event WHERE aggregate_id=$1 AND event_type='AgentActionRequested'`, created.TaskID).Scan(&obligation); err != nil {
		t.Fatal(err)
	}
	if len(obligation) != 1 || obligation[0] != "agent-action-dispatch" {
		t.Fatal("intent obligation lost", obligation)
	}
	t.Run("stop and action acceptance share one revision fence", func(t *testing.T) {
		raceCommand := application.Command{Principal: identity, Key: "race-create"}
		raceTask, e := svc.Handle(ctx, raceCommand, "CREATE", q)
		if e != nil {
			t.Fatal(e)
		}
		read, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		plan, e := svc.Repo.PlanTx(ctx, read, raceTask.TaskID, raceTask.PlanID)
		read.Rollback(ctx)
		if e != nil {
			t.Fatal(e)
		}
		raceCommand.TaskID = raceTask.TaskID
		raceCommand.Key = "race-approve"
		approved, e := svc.Handle(ctx, raceCommand, "APPROVE", application.Request{ExpectedRevision: 1, PlanID: plan.ID, ContentHash: plan.ContentHash, Phase: plan.Phase, Reason: "approve race"})
		if e != nil {
			t.Fatal(e)
		}
		start := make(chan struct{})
		result := make(chan error, 2)
		for _, kind := range []string{"REQUEST_ACTION", "STOP"} {
			go func(kind string) {
				<-start
				command := raceCommand
				command.Key = kind
				request := application.Request{ExpectedRevision: 2, Reason: "race"}
				if kind == "REQUEST_ACTION" {
					request.PlanID = plan.ID
					request.ContentHash = plan.ContentHash
					request.Phase = plan.Phase
					request.ApprovalID = *approved.ApprovalID
					request.Action = "DETECT"
				}
				_, e := svc.Handle(ctx, command, kind, request)
				result <- e
			}(kind)
		}
		close(start)
		first, second := <-result, <-result
		if !((first == nil && errors.Is(second, domain.ErrStale)) || (second == nil && errors.Is(first, domain.ErrStale))) {
			t.Fatalf("race outcomes: %v %v", first, second)
		}
		var state string
		if e = pool.QueryRow(ctx, `SELECT control_state FROM agent_task WHERE id=$1`, raceTask.TaskID).Scan(&state); e != nil {
			t.Fatal(e)
		}
		if state == "ACTIVE" {
			raceCommand.Key = "race-final-stop"
			if _, e = svc.Handle(ctx, raceCommand, "STOP", application.Request{ExpectedRevision: 3, Reason: "stop pending intent"}); e != nil {
				t.Fatal(e)
			}
		}
		var pending int
		if e = pool.QueryRow(ctx, `SELECT count(*) FROM agent_action_request a WHERE a.task_id=$1 AND NOT EXISTS(SELECT 1 FROM agent_action_disposition d WHERE d.action_id=a.id)`, raceTask.TaskID).Scan(&pending); e != nil || pending != 0 {
			t.Fatal("stopped task retained live intent", pending, e)
		}
	})
	// Missing route makes the entire new task + plan + receipt roll back.
	outbox.ConfigureAppendObligation(nil)
	c.TaskID = uuid.Nil
	c.Key = "rollback"
	if _, err = svc.Handle(ctx, c, "CREATE", q); err == nil {
		t.Fatal("missing proof route accepted")
	}
	var tasks int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_task WHERE workspace_id=$1`, ws).Scan(&tasks); err != nil || tasks != 2 {
		t.Fatal("partial task survived rollback", tasks, err)
	}
}
