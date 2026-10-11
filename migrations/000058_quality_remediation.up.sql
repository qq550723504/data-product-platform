-- #299/C1. Business associations only; existing Execution/Quality own attempts.
CREATE TABLE quality_remediation_detection (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, version_id uuid NOT NULL REFERENCES dataset_version(id),
 request_key varchar(128) NOT NULL, actor_id uuid NOT NULL, plan_hash char(64) NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(workspace_id,request_key)
);
CREATE TABLE quality_remediation_case (
 id uuid PRIMARY KEY REFERENCES quality_remediation_detection(id), workspace_id uuid NOT NULL,
 version_id uuid NOT NULL REFERENCES dataset_version(id), assessment_id uuid NOT NULL REFERENCES quality_result(id),
 finding_id uuid NOT NULL UNIQUE REFERENCES quality_finding(id),
 plan_ref text NOT NULL, plan_version text NOT NULL, plan_hash char(64) NOT NULL,
 detection_rule_id text NOT NULL, workflow_hash char(64) NOT NULL, input_name text NOT NULL,
 quarantine_reason text NOT NULL, recheck_rule_ref text NOT NULL, recheck_rule_hash char(64) NOT NULL,
 created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE quality_remediation_assignment (
 id uuid PRIMARY KEY, case_id uuid NOT NULL REFERENCES quality_remediation_case(id), actor_id uuid NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000), request_key varchar(128) NOT NULL,
 ordinal integer NOT NULL CHECK(ordinal>0),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(case_id,request_key), UNIQUE(case_id,ordinal)
);
CREATE TABLE quality_remediation_execution (
 id uuid PRIMARY KEY, case_id uuid NOT NULL REFERENCES quality_remediation_case(id),
 execution_id uuid NOT NULL UNIQUE REFERENCES execution(id), assignment_id uuid NOT NULL REFERENCES quality_remediation_assignment(id),
 ordinal integer NOT NULL CHECK(ordinal>0), actor_id uuid NOT NULL, request_key varchar(128) NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(case_id,request_key), UNIQUE(case_id,ordinal)
);
CREATE TABLE quality_remediation_recheck (
 id uuid PRIMARY KEY, case_id uuid NOT NULL REFERENCES quality_remediation_case(id),
 execution_link_id uuid NOT NULL REFERENCES quality_remediation_execution(id),
 quality_attempt_id uuid NOT NULL UNIQUE, actor_id uuid NOT NULL,
 request_key varchar(128) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(case_id,request_key)
);
CREATE TABLE quality_remediation_recheck_outcome (
 id uuid PRIMARY KEY REFERENCES quality_remediation_recheck(id),
 assessment_id uuid REFERENCES quality_result(id), outcome varchar(16) NOT NULL CHECK(outcome IN ('ASSESSED','FAILED')),
 error_code varchar(128) NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((outcome='ASSESSED')=(assessment_id IS NOT NULL)), CHECK(outcome<>'ASSESSED' OR error_code='')
);
CREATE TABLE quality_remediation_close (
 id uuid PRIMARY KEY, case_id uuid NOT NULL UNIQUE REFERENCES quality_remediation_case(id), actor_id uuid NOT NULL,
 request_key varchar(128) NOT NULL, reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION guard_quality_remediation() RETURNS trigger AS $$
DECLARE c quality_remediation_case; d quality_remediation_detection; e execution; previous execution;
 a quality_remediation_assignment; q quality_result; f quality_finding; rc quality_remediation_recheck;
 link quality_remediation_execution; dataset_workspace uuid; link_count integer;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'quality remediation facts are immutable'; END IF;
 IF TG_TABLE_NAME='quality_remediation_detection' THEN
  SELECT ds.workspace_id INTO dataset_workspace FROM dataset_version v JOIN dataset ds ON ds.id=v.dataset_id WHERE v.id=NEW.version_id;
  IF dataset_workspace IS DISTINCT FROM NEW.workspace_id THEN RAISE EXCEPTION 'remediation workspace mismatch'; END IF;
  RETURN NEW;
 END IF;
 IF TG_TABLE_NAME='quality_remediation_case' THEN
  SELECT * INTO d FROM quality_remediation_detection WHERE id=NEW.id FOR UPDATE;
  SELECT * INTO q FROM quality_result WHERE id=NEW.assessment_id;
  SELECT * INTO f FROM quality_finding WHERE id=NEW.finding_id;
  IF d.workspace_id IS DISTINCT FROM NEW.workspace_id OR d.version_id IS DISTINCT FROM NEW.version_id
   OR d.actor_id IS DISTINCT FROM NEW.created_by OR d.plan_hash IS DISTINCT FROM NEW.plan_hash
   OR q.workspace_id IS DISTINCT FROM NEW.workspace_id OR q.dataset_version_id IS DISTINCT FROM NEW.version_id
   OR f.result_id IS DISTINCT FROM q.id OR f.rule_id IS DISTINCT FROM NEW.detection_rule_id OR f.status IS DISTINCT FROM 'FAIL'
   OR NOT EXISTS(SELECT 1 FROM quality_assessment_attempt_outcome WHERE attempt_id=d.id AND assessment_id=q.id AND outcome='SUCCEEDED')
  THEN RAISE EXCEPTION 'remediation detection evidence mismatch'; END IF;
  RETURN NEW;
 END IF;
 IF TG_TABLE_NAME='quality_remediation_recheck_outcome' THEN
  SELECT * INTO rc FROM quality_remediation_recheck WHERE id=NEW.id;
  SELECT * INTO c FROM quality_remediation_case WHERE id=rc.case_id FOR UPDATE;
  SELECT * INTO link FROM quality_remediation_execution WHERE id=rc.execution_link_id;
  SELECT * INTO e FROM execution WHERE id=link.execution_id;
  IF NEW.assessment_id IS NOT NULL THEN
   SELECT * INTO q FROM quality_result WHERE id=NEW.assessment_id;
   IF q.workspace_id IS DISTINCT FROM c.workspace_id OR q.dataset_version_id IS DISTINCT FROM e.output_dataset_version_id
    OR q.rule_set_ref IS DISTINCT FROM c.recheck_rule_ref OR q.rule_set_content_sha256 IS DISTINCT FROM c.recheck_rule_hash
    OR NOT EXISTS(SELECT 1 FROM quality_assessment_attempt_outcome WHERE attempt_id=rc.quality_attempt_id AND assessment_id=q.id AND outcome='SUCCEEDED')
   THEN RAISE EXCEPTION 'remediation recheck evidence mismatch'; END IF;
  ELSIF NOT EXISTS(SELECT 1 FROM quality_assessment_attempt_outcome WHERE attempt_id=rc.quality_attempt_id AND outcome='FAILED')
   THEN RAISE EXCEPTION 'recheck failure requires authoritative quality outcome';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO c FROM quality_remediation_case WHERE id=NEW.case_id FOR UPDATE;
 IF c.id IS NULL THEN RAISE EXCEPTION 'remediation case missing'; END IF;
 IF EXISTS(SELECT 1 FROM quality_remediation_close WHERE case_id=c.id) THEN RAISE EXCEPTION 'remediation case manually closed'; END IF;
 IF TG_TABLE_NAME='quality_remediation_assignment' THEN
  SELECT count(*) INTO link_count FROM quality_remediation_assignment WHERE case_id=c.id;
  IF NEW.ordinal<>link_count+1 THEN RAISE EXCEPTION 'remediation assignment order mismatch'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO a FROM quality_remediation_assignment WHERE case_id=c.id ORDER BY ordinal DESC LIMIT 1;
 IF a.actor_id IS DISTINCT FROM NEW.actor_id THEN RAISE EXCEPTION 'remediation assignee mismatch'; END IF;
 IF TG_TABLE_NAME='quality_remediation_close' THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME='quality_remediation_execution' THEN
  SELECT * INTO e FROM execution WHERE id=NEW.execution_id;
  IF NEW.assignment_id IS DISTINCT FROM a.id OR e.workspace_id IS DISTINCT FROM c.workspace_id OR e.created_by IS DISTINCT FROM a.actor_id
   OR NOT EXISTS(SELECT 1 FROM workflow_version w WHERE w.id=e.workflow_version_id AND w.definition_sha256=c.workflow_hash)
   OR NOT EXISTS(SELECT 1 FROM execution_input i WHERE i.execution_id=e.id AND i.input_name=c.input_name AND i.dataset_version_id=c.version_id)
  THEN RAISE EXCEPTION 'remediation execution scope mismatch'; END IF;
  SELECT count(*) INTO link_count FROM quality_remediation_execution WHERE case_id=c.id;
  IF NEW.ordinal<>link_count+1 THEN RAISE EXCEPTION 'remediation attempt order mismatch'; END IF;
  SELECT x.* INTO previous FROM quality_remediation_execution l JOIN execution x ON x.id=l.execution_id WHERE l.case_id=c.id ORDER BY l.ordinal DESC LIMIT 1;
  IF previous.id IS NOT NULL AND (previous.status NOT IN ('FAILED','CANCELLED') OR e.retry_of_execution_id IS DISTINCT FROM previous.id)
   THEN RAISE EXCEPTION 'remediation retry requires latest failed execution'; END IF;
 ELSIF TG_TABLE_NAME='quality_remediation_recheck' THEN
  SELECT * INTO link FROM quality_remediation_execution WHERE id=NEW.execution_link_id;
  SELECT * INTO e FROM execution WHERE id=link.execution_id;
  IF link.case_id IS DISTINCT FROM c.id OR e.status IS DISTINCT FROM 'SUCCEEDED' OR e.output_dataset_version_id IS NULL
   THEN RAISE EXCEPTION 'remediation recheck requires successful exact output'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['quality_remediation_detection','quality_remediation_case','quality_remediation_assignment',
  'quality_remediation_execution','quality_remediation_recheck','quality_remediation_recheck_outcome','quality_remediation_close'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION guard_quality_remediation()', 'trg_'||t,t);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION guard_quality_remediation()', 'trg_'||t||'_truncate',t);
 END LOOP;
END; $$;
