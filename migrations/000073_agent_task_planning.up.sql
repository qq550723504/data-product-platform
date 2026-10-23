-- Q2 owns planning/approval facts only. There is deliberately no executor.
CREATE TABLE agent_task (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, kind text NOT NULL CHECK(kind='QUALITY_DIAGNOSIS'),
 created_by uuid NOT NULL, subject text NOT NULL CHECK(length(subject)>0), parent_task_id uuid REFERENCES agent_task(id),
 current_plan_revision_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 control_state text NOT NULL CHECK(control_state IN ('ACTIVE','PAUSED','STOPPED')),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(id,workspace_id)
);
CREATE INDEX agent_task_workspace ON agent_task(workspace_id,created_at DESC,id DESC);
CREATE TABLE agent_plan_revision (
 id uuid PRIMARY KEY, task_id uuid NOT NULL, workspace_id uuid NOT NULL,
 ordinal integer NOT NULL CHECK(ordinal>0), phase text NOT NULL CHECK(phase IN ('DIAGNOSIS','REPAIR')),
 source_version_id uuid NOT NULL REFERENCES dataset_version(id),
 source_assessment_id uuid REFERENCES quality_result(id), finding_id uuid REFERENCES quality_finding(id),
 case_id uuid REFERENCES quality_remediation_case(id),
 template_ref text NOT NULL, template_version text NOT NULL, template_hash text NOT NULL CHECK(template_hash ~ '^[0-9a-f]{64}$'),
 detection_rule_ref text NOT NULL, detection_rule_hash text NOT NULL CHECK(detection_rule_hash ~ '^[0-9a-f]{64}$'),
 workflow_version_id uuid REFERENCES workflow_version(id), workflow_hash text NOT NULL CHECK(workflow_hash ~ '^[0-9a-f]{64}$'),
 output_dataset_id uuid REFERENCES dataset(id), target_period text NOT NULL,
 recheck_rule_ref text NOT NULL, recheck_rule_hash text NOT NULL CHECK(recheck_rule_hash ~ '^[0-9a-f]{64}$'),
 goal text NOT NULL CHECK(length(goal) BETWEEN 1 AND 300), purpose_note text NOT NULL CHECK(length(purpose_note)<=1000),
 acceptance_text text NOT NULL CHECK(length(acceptance_text) BETWEEN 1 AND 2000),
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'), canonical_content text NOT NULL,
 frozen boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(task_id,workspace_id) REFERENCES agent_task(id,workspace_id), UNIQUE(task_id,ordinal), UNIQUE(id,task_id),
 CHECK((phase='DIAGNOSIS' AND source_assessment_id IS NULL AND finding_id IS NULL AND case_id IS NULL AND workflow_version_id IS NULL AND output_dataset_id IS NULL AND target_period='')
    OR (phase='REPAIR' AND source_assessment_id IS NOT NULL AND finding_id IS NOT NULL AND case_id IS NOT NULL AND workflow_version_id IS NOT NULL AND output_dataset_id IS NOT NULL AND length(target_period)>0))
);
ALTER TABLE agent_task ADD CONSTRAINT agent_task_current_plan FOREIGN KEY(current_plan_revision_id,id)
 REFERENCES agent_plan_revision(id,task_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE agent_plan_input (
 plan_revision_id uuid NOT NULL REFERENCES agent_plan_revision(id), input_name text NOT NULL CHECK(length(input_name) BETWEEN 1 AND 128),
 dataset_version_id uuid NOT NULL REFERENCES dataset_version(id), required boolean NOT NULL CHECK(required),
 receipt_id uuid REFERENCES ingestion_receipt(id), resolution_job_id uuid REFERENCES entity_match_job(id),
 PRIMARY KEY(plan_revision_id,input_name), CHECK(receipt_id IS NULL OR resolution_job_id IS NULL)
);
CREATE TABLE agent_plan_action (
 plan_revision_id uuid NOT NULL REFERENCES agent_plan_revision(id), ordinal integer NOT NULL CHECK(ordinal>0),
 command_kind text NOT NULL CHECK(command_kind IN ('DETECT','ASSIGN_SELF','EXECUTE_REPAIR','RECHECK')),
 approval_phase text NOT NULL CHECK(approval_phase IN ('DIAGNOSIS','REPAIR')), PRIMARY KEY(plan_revision_id,ordinal),
 UNIQUE(plan_revision_id,command_kind)
);
CREATE TABLE agent_approval (
 id uuid PRIMARY KEY, task_id uuid NOT NULL REFERENCES agent_task(id), plan_revision_id uuid NOT NULL,
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'), phase text NOT NULL CHECK(phase IN ('DIAGNOSIS','REPAIR')),
 actor_id uuid NOT NULL, subject text NOT NULL, scope_hash text NOT NULL CHECK(scope_hash ~ '^[0-9a-f]{64}$'),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(plan_revision_id,task_id) REFERENCES agent_plan_revision(id,task_id), UNIQUE(plan_revision_id,phase)
);
CREATE TABLE agent_action_request (
 id uuid PRIMARY KEY, task_id uuid NOT NULL, workspace_id uuid NOT NULL, plan_revision_id uuid NOT NULL,
 approval_id uuid NOT NULL REFERENCES agent_approval(id), ordinal integer NOT NULL,
 command_kind text NOT NULL, request_key varchar(128) NOT NULL, fingerprint text NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 business_request_key varchar(128) NOT NULL UNIQUE, dispatch_state text NOT NULL CHECK(dispatch_state='REQUESTED'),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(task_id,workspace_id) REFERENCES agent_task(id,workspace_id),
 FOREIGN KEY(plan_revision_id,task_id) REFERENCES agent_plan_revision(id,task_id),
 FOREIGN KEY(plan_revision_id,ordinal) REFERENCES agent_plan_action(plan_revision_id,ordinal), UNIQUE(plan_revision_id,ordinal)
);
CREATE TABLE agent_action_disposition (
 id uuid PRIMARY KEY, task_id uuid NOT NULL REFERENCES agent_task(id), action_id uuid NOT NULL UNIQUE REFERENCES agent_action_request(id),
 disposition text NOT NULL CHECK(disposition='CANCELLED'), actor_id uuid NOT NULL, subject text NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE agent_command_receipt (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, command_kind text NOT NULL, target_id uuid NOT NULL,
 request_key varchar(128) NOT NULL CHECK(length(request_key)>0), fingerprint text NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 actor_id uuid NOT NULL, subject text NOT NULL, task_id uuid NOT NULL REFERENCES agent_task(id), plan_revision_id uuid NOT NULL,
 approval_id uuid REFERENCES agent_approval(id), action_id uuid REFERENCES agent_action_request(id), task_revision bigint NOT NULL CHECK(task_revision>0),
 created_at timestamptz NOT NULL DEFAULT now(), FOREIGN KEY(plan_revision_id,task_id) REFERENCES agent_plan_revision(id,task_id),
 UNIQUE(workspace_id,command_kind,target_id,request_key)
);

CREATE FUNCTION agent_plan_content(plan_id uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('schemaVersion',1,'workspaceId',p.workspace_id,'taskId',p.task_id,'ordinal',p.ordinal,'phase',p.phase,
 'sourceVersionId',p.source_version_id,'sourceAssessmentId',p.source_assessment_id,'findingId',p.finding_id,'caseId',p.case_id,
 'templateRef',p.template_ref,'templateVersion',p.template_version,'templateHash',p.template_hash,
 'detectionRuleRef',p.detection_rule_ref,'detectionRuleHash',p.detection_rule_hash,'workflowVersionId',p.workflow_version_id,'workflowHash',p.workflow_hash,
 'outputDatasetId',p.output_dataset_id,'targetPeriod',p.target_period,'recheckRuleRef',p.recheck_rule_ref,'recheckRuleHash',p.recheck_rule_hash,
 'goal',p.goal,'purposeNote',p.purpose_note,'acceptanceText',p.acceptance_text,
 'inputs',coalesce((SELECT jsonb_agg(jsonb_build_object('inputName',i.input_name,'datasetVersionId',i.dataset_version_id,'required',i.required,
   'receiptId',i.receipt_id,'resolutionJobId',i.resolution_job_id) ORDER BY i.input_name COLLATE "C") FROM agent_plan_input i WHERE i.plan_revision_id=p.id),'[]'::jsonb),
 'actions',coalesce((SELECT jsonb_agg(jsonb_build_object('ordinal',a.ordinal,'commandKind',a.command_kind,'approvalPhase',a.approval_phase) ORDER BY a.ordinal)
   FROM agent_plan_action a WHERE a.plan_revision_id=p.id),'[]'::jsonb))
 FROM agent_plan_revision p WHERE p.id=plan_id
$$;

CREATE FUNCTION guard_agent_plan_member() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE is_frozen boolean;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'Agent plan members are append-only'; END IF;
 -- This is the same parent lock acquired by finalization, including when a
 -- transaction began before the plan was frozen. No unlocked status read.
 SELECT frozen INTO STRICT is_frozen FROM agent_plan_revision WHERE id=NEW.plan_revision_id FOR UPDATE;
 IF is_frozen THEN RAISE EXCEPTION 'Agent plan is frozen'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_input_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_plan_input FOR EACH ROW EXECUTE FUNCTION guard_agent_plan_member();
CREATE TRIGGER agent_action_member_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_plan_action FOR EACH ROW EXECUTE FUNCTION guard_agent_plan_member();

CREATE FUNCTION guard_agent_plan() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE kinds text[]; ordinals integer[];
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Agent plan history is immutable'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.frozen THEN RAISE EXCEPTION 'Agent plan must be assembled before freezing'; END IF;
  RETURN NEW;
 END IF;
 IF OLD.frozen OR NOT NEW.frozen THEN RAISE EXCEPTION 'Agent plan supports only atomic finalization'; END IF;
 IF (to_jsonb(NEW)-'frozen') IS DISTINCT FROM (to_jsonb(OLD)-'frozen') THEN RAISE EXCEPTION 'Agent freeze cannot change content'; END IF;
 IF ROW(NEW.id,NEW.task_id,NEW.workspace_id,NEW.ordinal) IS DISTINCT FROM ROW(OLD.id,OLD.task_id,OLD.workspace_id,OLD.ordinal)
  THEN RAISE EXCEPTION 'Agent plan identity is immutable'; END IF;
 -- All members are visible here while holding their common parent lock.
 IF NEW.canonical_content::jsonb IS DISTINCT FROM agent_plan_content(NEW.id)
  OR encode(digest(NEW.canonical_content,'sha256'),'hex')<>NEW.content_hash THEN RAISE EXCEPTION 'Agent plan content/hash mismatch'; END IF;
 IF NOT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id
  WHERE v.id=NEW.source_version_id AND d.workspace_id=NEW.workspace_id AND d.dataset_type='RAW' AND v.status='READY')
  THEN RAISE EXCEPTION 'Agent source scope invalid'; END IF;
 IF EXISTS(SELECT 1 FROM agent_plan_input i JOIN dataset_version v ON v.id=i.dataset_version_id JOIN dataset d ON d.id=v.dataset_id
  WHERE i.plan_revision_id=NEW.id AND d.workspace_id<>NEW.workspace_id) THEN RAISE EXCEPTION 'Agent input workspace mismatch'; END IF;
 SELECT array_agg(command_kind ORDER BY ordinal) INTO kinds FROM agent_plan_action WHERE plan_revision_id=NEW.id;
 SELECT array_agg(ordinal ORDER BY ordinal) INTO ordinals FROM agent_plan_action WHERE plan_revision_id=NEW.id;
 IF EXISTS(SELECT 1 FROM agent_plan_action WHERE plan_revision_id=NEW.id AND approval_phase<>NEW.phase) THEN RAISE EXCEPTION 'Agent action phase mismatch'; END IF;
 IF NEW.phase='DIAGNOSIS' THEN
  IF kinds IS DISTINCT FROM ARRAY['DETECT']::text[] OR ordinals IS DISTINCT FROM ARRAY[1] OR (SELECT count(*) FROM agent_plan_input WHERE plan_revision_id=NEW.id)<>1
   OR NOT EXISTS(SELECT 1 FROM agent_plan_input WHERE plan_revision_id=NEW.id AND dataset_version_id=NEW.source_version_id AND receipt_id IS NULL AND resolution_job_id IS NULL)
   THEN RAISE EXCEPTION 'Agent diagnosis membership invalid'; END IF;
 ELSE
  IF kinds IS DISTINCT FROM ARRAY['ASSIGN_SELF','EXECUTE_REPAIR','RECHECK']::text[] OR ordinals IS DISTINCT FROM ARRAY[1,2,3]
   OR NOT EXISTS(SELECT 1 FROM agent_plan_input WHERE plan_revision_id=NEW.id)
   OR EXISTS(SELECT 1 FROM agent_plan_input WHERE plan_revision_id=NEW.id AND (receipt_id IS NULL)=(resolution_job_id IS NULL))
   THEN RAISE EXCEPTION 'Agent repair membership invalid'; END IF;
  IF NOT EXISTS(SELECT 1 FROM quality_remediation_case c WHERE c.id=NEW.case_id AND c.workspace_id=NEW.workspace_id
   AND c.version_id=NEW.source_version_id AND c.assessment_id=NEW.source_assessment_id AND c.finding_id=NEW.finding_id
   AND c.plan_ref=NEW.template_ref AND c.plan_version=NEW.template_version AND c.plan_hash=NEW.template_hash AND c.workflow_hash=NEW.workflow_hash
   AND c.recheck_rule_ref=NEW.recheck_rule_ref AND c.recheck_rule_hash=NEW.recheck_rule_hash)
   OR NOT EXISTS(SELECT 1 FROM workflow_version w JOIN workflow wf ON wf.id=w.workflow_id WHERE w.id=NEW.workflow_version_id AND wf.workspace_id=NEW.workspace_id AND w.definition_sha256=NEW.workflow_hash)
   OR NOT EXISTS(SELECT 1 FROM dataset WHERE id=NEW.output_dataset_id AND workspace_id=NEW.workspace_id AND dataset_type='CURATED')
   THEN RAISE EXCEPTION 'Agent repair reference mismatch'; END IF;
  IF EXISTS(SELECT 1 FROM agent_plan_input i WHERE i.plan_revision_id=NEW.id AND i.receipt_id IS NOT NULL AND NOT EXISTS(
   SELECT 1 FROM ingestion_receipt_slot s JOIN ingestion_receipt r ON r.id=s.receipt_id JOIN ingestion_operation o ON o.id=r.operation_id
   WHERE s.receipt_id=i.receipt_id AND s.code=i.input_name AND s.version_id=i.dataset_version_id AND o.workspace_id=NEW.workspace_id AND o.state='ACCEPTED'))
   OR EXISTS(SELECT 1 FROM agent_plan_input i WHERE i.plan_revision_id=NEW.id AND i.resolution_job_id IS NOT NULL AND NOT EXISTS(
   SELECT 1 FROM entity_match_job j JOIN dataset_version v ON v.id=j.output_dataset_version_id WHERE j.id=i.resolution_job_id
   AND j.workspace_id=NEW.workspace_id AND j.status='SUCCEEDED' AND j.output_dataset_version_id=i.dataset_version_id AND v.generated_by_entity_match_job_id=j.id))
   THEN RAISE EXCEPTION 'Agent repair input provenance mismatch'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_plan_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_plan_revision FOR EACH ROW EXECUTE FUNCTION guard_agent_plan();

CREATE FUNCTION agent_plan_must_be_frozen() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM agent_plan_revision WHERE id=NEW.id AND frozen) THEN RAISE EXCEPTION 'Agent DRAFT cannot commit'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER agent_plan_atomic AFTER INSERT OR UPDATE ON agent_plan_revision DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION agent_plan_must_be_frozen();

CREATE FUNCTION guard_agent_task() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Agent task history cannot be deleted'; END IF;
 IF TG_OP='UPDATE' THEN
  IF ROW(NEW.id,NEW.workspace_id,NEW.kind,NEW.created_by,NEW.subject,NEW.parent_task_id,NEW.created_at) IS DISTINCT FROM
     ROW(OLD.id,OLD.workspace_id,OLD.kind,OLD.created_by,OLD.subject,OLD.parent_task_id,OLD.created_at)
   OR NEW.revision<>OLD.revision+1 OR OLD.control_state='STOPPED' THEN RAISE EXCEPTION 'Agent task transition invalid'; END IF;
 ELSE
  IF NEW.revision<>1 OR NEW.control_state<>'ACTIVE' THEN RAISE EXCEPTION 'Agent task initial state invalid'; END IF;
 END IF;
 IF NEW.parent_task_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_task WHERE id=NEW.parent_task_id AND workspace_id=NEW.workspace_id)
  THEN RAISE EXCEPTION 'Agent parent workspace mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_task_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_task FOR EACH ROW EXECUTE FUNCTION guard_agent_task();

CREATE FUNCTION guard_agent_fact() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t agent_task; p agent_plan_revision; a agent_approval;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'Agent historical fact is immutable'; END IF;
 SELECT * INTO STRICT t FROM agent_task WHERE id=NEW.task_id FOR UPDATE;
 IF TG_TABLE_NAME='agent_action_disposition' THEN
  IF NEW.actor_id<>t.created_by OR NEW.subject<>t.subject OR t.control_state<>'STOPPED'
   OR NOT EXISTS(SELECT 1 FROM agent_action_request WHERE id=NEW.action_id AND task_id=t.id)
   THEN RAISE EXCEPTION 'Agent disposition scope invalid'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO STRICT p FROM agent_plan_revision WHERE id=NEW.plan_revision_id AND task_id=t.id;
 IF NOT p.frozen THEN RAISE EXCEPTION 'Agent fact requires frozen plan'; END IF;
 IF TG_TABLE_NAME='agent_command_receipt' THEN
  IF NEW.workspace_id<>t.workspace_id OR NEW.actor_id<>t.created_by OR NEW.subject<>t.subject
   OR (NEW.approval_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_approval WHERE id=NEW.approval_id AND task_id=t.id AND plan_revision_id=p.id))
   OR (NEW.action_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM agent_action_request WHERE id=NEW.action_id AND task_id=t.id AND plan_revision_id=p.id))
   THEN RAISE EXCEPTION 'Agent receipt scope invalid'; END IF;
  RETURN NEW;
 END IF;
 IF t.control_state<>'ACTIVE' OR t.current_plan_revision_id<>p.id THEN RAISE EXCEPTION 'Agent current plan/state mismatch'; END IF;
 IF TG_TABLE_NAME='agent_approval' THEN
  IF NEW.actor_id<>t.created_by OR NEW.subject<>t.subject OR NEW.content_hash<>p.content_hash OR NEW.phase<>p.phase
   THEN RAISE EXCEPTION 'Agent approval scope invalid'; END IF;
 ELSE
  SELECT * INTO STRICT a FROM agent_approval WHERE id=NEW.approval_id AND task_id=t.id AND plan_revision_id=p.id;
  IF NEW.workspace_id<>t.workspace_id OR a.content_hash<>p.content_hash OR NOT EXISTS(SELECT 1 FROM agent_plan_action
   WHERE plan_revision_id=p.id AND ordinal=NEW.ordinal AND command_kind=NEW.command_kind AND approval_phase=a.phase)
   THEN RAISE EXCEPTION 'Agent intent scope invalid'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_approval_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_approval FOR EACH ROW EXECUTE FUNCTION guard_agent_fact();
CREATE TRIGGER agent_intent_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_action_request FOR EACH ROW EXECUTE FUNCTION guard_agent_fact();
CREATE TRIGGER agent_disposition_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_action_disposition FOR EACH ROW EXECUTE FUNCTION guard_agent_fact();
CREATE TRIGGER agent_receipt_guard BEFORE INSERT OR UPDATE OR DELETE ON agent_command_receipt FOR EACH ROW EXECUTE FUNCTION guard_agent_fact();
