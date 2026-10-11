-- Additive exact-version execution/coverage facts. No historical locator/status rewrite.
-- This is an explicit synthetic test policy, not a statutory/business policy.
CREATE TABLE lifecycle_fixture_disposal_basis (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, run_id uuid NOT NULL UNIQUE,
 inventory_plan_id uuid NOT NULL, approved_plan_id uuid NOT NULL,
 inventory_hash varchar(64) NOT NULL, approved_hash varchar(64) NOT NULL,
 basis_ref varchar(255) NOT NULL, starts_at timestamptz NOT NULL,
 retain_until timestamptz NOT NULL, dispose_due_at timestamptz NOT NULL,
 start_event_id uuid NOT NULL REFERENCES outbox_event(id),
 CHECK(starts_at=retain_until AND starts_at=dispose_due_at),
 FOREIGN KEY(workspace_id,inventory_plan_id) REFERENCES lifecycle_disposition_plan(workspace_id,id),
 FOREIGN KEY(workspace_id,approved_plan_id) REFERENCES lifecycle_disposition_plan(workspace_id,id)
);
CREATE TABLE lifecycle_provider_send_claim (
 attempt_id uuid PRIMARY KEY REFERENCES lifecycle_provider_attempt(id),
 workspace_id uuid NOT NULL, claimed_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,attempt_id) REFERENCES lifecycle_provider_attempt(workspace_id,id)
);
CREATE FUNCTION lifecycle_send_claim_guard() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM lifecycle_provider_attempt a WHERE a.id=NEW.attempt_id AND a.workspace_id=NEW.workspace_id AND a.started_at<=NEW.claimed_at)
 OR EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=NEW.attempt_id) THEN
  RAISE EXCEPTION 'send claim requires unobserved exact physical start';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_send_claim_scope BEFORE INSERT ON lifecycle_provider_send_claim FOR EACH ROW EXECUTE FUNCTION lifecycle_send_claim_guard();
CREATE TABLE lifecycle_copy_absence_receipt (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, plan_id uuid NOT NULL, copy_id uuid NOT NULL,
 dispatch_id uuid NOT NULL UNIQUE, inventory_call_id uuid NOT NULL,
 plan_hash varchar(64) NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'),
 response_sha256 varchar(64) NOT NULL CHECK(response_sha256 ~ '^[0-9a-f]{64}$'),
 observed_at timestamptz NOT NULL, confirmed_at timestamptz NOT NULL CHECK(confirmed_at>=observed_at),
 dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 FOREIGN KEY(workspace_id,dispatch_id,plan_id,copy_id) REFERENCES lifecycle_disposition_dispatch(workspace_id,id,plan_id,copy_id),
 FOREIGN KEY(workspace_id,inventory_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 UNIQUE(workspace_id,plan_id,copy_id)
);
CREATE FUNCTION lifecycle_absence_guard() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM lifecycle_disposition_plan p JOIN lifecycle_disposition_dispatch d ON d.plan_id=p.id
  JOIN lifecycle_storage_call c ON c.id=NEW.inventory_call_id
  JOIN lifecycle_storage_observation o ON o.call_id=c.id
  JOIN lifecycle_fixture_copy target ON target.id=NEW.copy_id
  JOIN lifecycle_fixture_copy scanned ON scanned.id=c.copy_id
  JOIN lifecycle_fixture_namespace tn ON tn.id=target.namespace_id
  JOIN lifecycle_fixture_namespace sn ON sn.id=scanned.namespace_id
  WHERE p.workspace_id=NEW.workspace_id AND p.id=NEW.plan_id AND p.status='FINALIZED' AND p.content_hash=NEW.plan_hash
   AND d.id=NEW.dispatch_id AND c.workspace_id=NEW.workspace_id AND c.operation='LIST_NAMESPACE_VERSIONS'
   AND sn.provider_instance_id=tn.provider_instance_id AND sn.incarnation_id=tn.incarnation_id AND sn.namespace=tn.namespace
   AND o.http_status=200 AND o.failure='' AND o.invoked_at IS NOT NULL AND o.response_sha256=NEW.response_sha256
   AND o.observed_at=NEW.observed_at AND o.observed_at>=d.dispatched_at) THEN
  RAISE EXCEPTION 'absence receipt requires scoped successful post-dispatch inventory';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_absence_scope BEFORE INSERT ON lifecycle_copy_absence_receipt FOR EACH ROW EXECUTE FUNCTION lifecycle_absence_guard();
CREATE TABLE lifecycle_plan_completion (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, plan_id uuid NOT NULL UNIQUE,
 plan_hash varchar(64) NOT NULL, target_count integer NOT NULL CHECK(target_count BETWEEN 1 AND 64),
 confirmed_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 FOREIGN KEY(workspace_id,plan_id) REFERENCES lifecycle_disposition_plan(workspace_id,id)
);
CREATE FUNCTION lifecycle_completion_guard() RETURNS trigger AS $$
DECLARE p lifecycle_disposition_plan;
BEGIN
 SELECT * INTO STRICT p FROM lifecycle_disposition_plan WHERE id=NEW.plan_id AND workspace_id=NEW.workspace_id FOR UPDATE;
 IF p.status<>'FINALIZED' OR p.content_hash<>NEW.plan_hash OR (SELECT count(*) FROM lifecycle_disposition_plan_target WHERE plan_id=p.id)<>NEW.target_count
  OR (SELECT count(*) FROM lifecycle_copy_absence_receipt WHERE plan_id=p.id)<>NEW.target_count
  OR EXISTS(SELECT 1 FROM lifecycle_disposition_plan_target t WHERE t.plan_id=p.id AND NOT EXISTS(
    SELECT 1 FROM lifecycle_copy_absence_receipt r WHERE r.plan_id=p.id AND r.copy_id=t.copy_id AND r.confirmed_at<=NEW.confirmed_at)) THEN
  RAISE EXCEPTION 'completion requires exact frozen receipt membership';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_completion_scope BEFORE INSERT ON lifecycle_plan_completion FOR EACH ROW EXECUTE FUNCTION lifecycle_completion_guard();
DO $$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['lifecycle_fixture_disposal_basis','lifecycle_provider_send_claim','lifecycle_copy_absence_receipt','lifecycle_plan_completion'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_immutable',name);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_no_truncate',name);
 END LOOP;
END $$;
