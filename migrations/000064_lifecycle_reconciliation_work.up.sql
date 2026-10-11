-- Durable recovery requests reuse the platform outbox and Asynq runtime.
-- A completed recovery request never implies that an UNKNOWN delete stopped.
CREATE TABLE lifecycle_reconciliation_work (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, attempt_id uuid NOT NULL,
 basis_ref varchar(255) NOT NULL CHECK(length(basis_ref)>0),
 requested_at timestamptz NOT NULL, actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,attempt_id) REFERENCES lifecycle_provider_attempt(workspace_id,id),
 UNIQUE(workspace_id,id)
);
CREATE INDEX lifecycle_reconciliation_work_attempt ON lifecycle_reconciliation_work(workspace_id,attempt_id);
CREATE TABLE lifecycle_reconciliation_run (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, work_id uuid NOT NULL,
 started_at timestamptz NOT NULL, actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,work_id) REFERENCES lifecycle_reconciliation_work(workspace_id,id),
 UNIQUE(workspace_id,id,work_id)
);
CREATE TABLE lifecycle_reconciliation_result (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, work_id uuid NOT NULL, run_id uuid NOT NULL,
 outcome varchar(32) NOT NULL CHECK(outcome IN('UNKNOWN_PRESENT','UNKNOWN_PROVIDER','BLOCKED_AUTHORITY','ABSENT_PARTIAL','ABSENT_VERIFIED','ALREADY_RESOLVED')),
 terminal boolean NOT NULL,
 inventory_call_id uuid, stat_call_id uuid,
 absence_receipt_id uuid REFERENCES lifecycle_copy_absence_receipt(id),
 completion_id uuid REFERENCES lifecycle_plan_completion(id),
 observed_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,run_id,work_id) REFERENCES lifecycle_reconciliation_run(workspace_id,id,work_id),
 FOREIGN KEY(workspace_id,inventory_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 FOREIGN KEY(workspace_id,stat_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 CHECK(terminal=(outcome<>'UNKNOWN_PROVIDER')),
 CHECK(outcome<>'UNKNOWN_PRESENT' OR (inventory_call_id IS NOT NULL AND stat_call_id IS NOT NULL)),
 CHECK(outcome NOT IN('ABSENT_PARTIAL','ABSENT_VERIFIED') OR absence_receipt_id IS NOT NULL),
 CHECK(outcome<>'ABSENT_VERIFIED' OR completion_id IS NOT NULL)
);
CREATE UNIQUE INDEX lifecycle_reconciliation_one_terminal ON lifecycle_reconciliation_result(work_id) WHERE terminal;
CREATE FUNCTION lifecycle_reconciliation_scope_guard() RETURNS trigger AS $$
DECLARE a lifecycle_provider_attempt;
BEGIN
 IF TG_TABLE_NAME='lifecycle_reconciliation_run' THEN
  PERFORM id FROM lifecycle_reconciliation_work WHERE id=NEW.work_id AND workspace_id=NEW.workspace_id AND requested_at<=NEW.started_at FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'recovery run requires scoped durable request'; END IF;
 ELSE
  SELECT p.* INTO STRICT a FROM lifecycle_reconciliation_work w JOIN lifecycle_provider_attempt p ON p.id=w.attempt_id WHERE w.id=NEW.work_id AND w.workspace_id=NEW.workspace_id;
  IF NEW.inventory_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.id=NEW.inventory_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_id=a.copy_id AND c.operation='LIST_NAMESPACE_VERSIONS' AND o.http_status=200 AND o.failure='' AND o.invoked_at IS NOT NULL AND o.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked recovery inventory'; END IF;
  IF NEW.stat_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.id=NEW.stat_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_id=a.copy_id AND c.operation='STAT_EXACT' AND o.http_status=200 AND o.failure='' AND o.invoked_at IS NOT NULL AND o.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked recovery presence'; END IF;
  IF NEW.absence_receipt_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE id=NEW.absence_receipt_id AND workspace_id=NEW.workspace_id AND dispatch_id=a.dispatch_id) THEN RAISE EXCEPTION 'recovery receipt scope mismatch'; END IF;
  IF NEW.completion_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_plan_completion WHERE id=NEW.completion_id AND workspace_id=NEW.workspace_id AND plan_id=a.plan_id) THEN RAISE EXCEPTION 'recovery completion scope mismatch'; END IF;
  IF NOT EXISTS(SELECT 1 FROM lifecycle_reconciliation_run WHERE id=NEW.run_id AND started_at<=NEW.observed_at) THEN RAISE EXCEPTION 'recovery observation precedes run'; END IF;
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_reconciliation_run_scope BEFORE INSERT ON lifecycle_reconciliation_run FOR EACH ROW EXECUTE FUNCTION lifecycle_reconciliation_scope_guard();
CREATE TRIGGER lifecycle_reconciliation_result_scope BEFORE INSERT ON lifecycle_reconciliation_result FOR EACH ROW EXECUTE FUNCTION lifecycle_reconciliation_scope_guard();
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['lifecycle_reconciliation_work','lifecycle_reconciliation_run','lifecycle_reconciliation_result'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_immutable',n);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_no_truncate',n);
 END LOOP;
END $$;
