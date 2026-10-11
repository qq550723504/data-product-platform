-- #299/C2 finite source observations; no DatasetVersion/QA/Certification writes.
CREATE TABLE source_schema_request (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL,
 resource_id uuid NOT NULL REFERENCES data_resource(id), actor_id uuid NOT NULL,
 authorization_id uuid NOT NULL REFERENCES data_authorization(id), consumer_ref text NOT NULL,
 purpose text NOT NULL, binding_id uuid NOT NULL REFERENCES resource_binding(id),
 external_id text NOT NULL, external_fqn text NOT NULL, rule_ref text NOT NULL,
 rule_version text NOT NULL, rule_hash char(64) NOT NULL,
 plan_hash char(64) NOT NULL, plan jsonb NOT NULL, request_key varchar(128) NOT NULL,
 requested_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(workspace_id,request_key),
 CHECK(rule_hash ~ '^[0-9a-f]{64}$' AND plan_hash ~ '^[0-9a-f]{64}$'),
 CHECK(plan @> jsonb_build_object('enabled',true,'workspaceId',workspace_id,'resourceId',resource_id,
  'actorId',actor_id,'authorizationId',authorization_id,'consumerRef',consumer_ref,'purpose',purpose,
  'externalFqn',external_fqn,'ruleRef',rule_ref,'ruleVersion',rule_version,'ruleHash',rule_hash)),
 CHECK(jsonb_typeof(plan->'columns')='array' AND jsonb_array_length(plan->'columns') BETWEEN 1 AND 100),
 CHECK(plan->>'connectorHash' ~ '^[0-9a-f]{64}$')
);
CREATE TABLE source_schema_attempt (
 id uuid PRIMARY KEY, request_id uuid NOT NULL REFERENCES source_schema_request(id),
 workspace_id uuid NOT NULL, started_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE source_schema_observation (
 id uuid PRIMARY KEY, request_id uuid NOT NULL REFERENCES source_schema_request(id),
 attempt_id uuid UNIQUE REFERENCES source_schema_attempt(id),
 status text NOT NULL CHECK(status IN ('MATCH','DRIFT','UNAVAILABLE','DENIED','INTERRUPTED')),
 reason text NOT NULL, columns jsonb NOT NULL, schema_hash text NOT NULL,
 started_at timestamptz, completed_at timestamptz, recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK ((status IN ('MATCH','DRIFT'))=(schema_hash<>'')),
 CHECK(status NOT IN ('MATCH','DRIFT') OR (attempt_id IS NOT NULL AND started_at IS NOT NULL AND completed_at>=started_at))
);
CREATE TABLE source_schema_handling (
 id uuid PRIMARY KEY, observation_id uuid NOT NULL REFERENCES source_schema_observation(id),
 actor_id uuid NOT NULL, reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 request_key varchar(128) NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(observation_id,request_key)
);
CREATE FUNCTION source_schema_fact_guard() RETURNS trigger AS $$
DECLARE r source_schema_request; a source_schema_attempt;
BEGIN
 IF TG_OP <> 'INSERT' THEN RAISE EXCEPTION 'source schema history is immutable'; END IF;
 IF TG_TABLE_NAME='source_schema_attempt' THEN
  SELECT * INTO r FROM source_schema_request WHERE id=NEW.request_id;
  IF r.workspace_id IS DISTINCT FROM NEW.workspace_id THEN RAISE EXCEPTION 'schema attempt crosses workspace'; END IF;
 ELSIF TG_TABLE_NAME='source_schema_observation' THEN
  IF NEW.attempt_id IS NOT NULL THEN
   SELECT * INTO a FROM source_schema_attempt WHERE id=NEW.attempt_id;
   IF a.request_id IS DISTINCT FROM NEW.request_id THEN RAISE EXCEPTION 'schema observation crosses request'; END IF;
  END IF;
 ELSIF TG_TABLE_NAME='source_schema_request' THEN
  IF NOT EXISTS(SELECT 1 FROM data_resource WHERE id=NEW.resource_id AND workspace_id=NEW.workspace_id)
   OR NOT EXISTS(SELECT 1 FROM resource_binding WHERE id=NEW.binding_id AND resource_id=NEW.resource_id AND external_id=NEW.external_id AND external_fqn=NEW.external_fqn)
   OR NOT EXISTS(SELECT 1 FROM data_authorization WHERE id=NEW.authorization_id AND workspace_id=NEW.workspace_id) THEN RAISE EXCEPTION 'schema request crosses ownership boundary'; END IF;
 ELSIF TG_TABLE_NAME='source_schema_handling' THEN
  IF NOT EXISTS(SELECT 1 FROM source_schema_observation o JOIN source_schema_request rq ON rq.id=o.request_id
   WHERE o.id=NEW.observation_id AND o.status='DRIFT' AND rq.actor_id=NEW.actor_id) THEN RAISE EXCEPTION 'invalid schema handling responsibility'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER source_schema_request_fact BEFORE INSERT OR UPDATE OR DELETE ON source_schema_request FOR EACH ROW EXECUTE FUNCTION source_schema_fact_guard();
CREATE TRIGGER source_schema_attempt_fact BEFORE INSERT OR UPDATE OR DELETE ON source_schema_attempt FOR EACH ROW EXECUTE FUNCTION source_schema_fact_guard();
CREATE TRIGGER source_schema_observation_fact BEFORE INSERT OR UPDATE OR DELETE ON source_schema_observation FOR EACH ROW EXECUTE FUNCTION source_schema_fact_guard();
CREATE TRIGGER source_schema_handling_fact BEFORE INSERT OR UPDATE OR DELETE ON source_schema_handling FOR EACH ROW EXECUTE FUNCTION source_schema_fact_guard();
CREATE INDEX source_schema_resource_history ON source_schema_request(workspace_id,resource_id,requested_at DESC);
ALTER TABLE cost_allocation ADD COLUMN source_schema_attempt_id uuid REFERENCES source_schema_attempt(id);
CREATE UNIQUE INDEX source_schema_cost ON cost_allocation(source_schema_attempt_id) WHERE source_schema_attempt_id IS NOT NULL;
-- Extend rather than replace the current shared CostAllocation contract. This
-- preserves any preceding typed subjects (including #307 when integrated).
DO $$
DECLARE definition text; guard text;
BEGIN
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='cost_allocation'::regclass AND conname='ck_cost_allocation_single_subject';
 IF definition NOT LIKE '%num_nonnulls(%' THEN RAISE EXCEPTION 'unsupported cost subject contract'; END IF;
 ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
 EXECUTE 'ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject ' || replace(definition,'num_nonnulls(','num_nonnulls(source_schema_attempt_id,');
 SELECT pg_get_functiondef('guard_cost_allocation_workspace()'::regprocedure) INTO definition;
 definition:=replace(definition,E'\r\n',E'\n');
 guard := E'\n-- source-schema guard begin\n IF NEW.source_schema_attempt_id IS NOT NULL THEN\n  SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;\n  SELECT workspace_id INTO subject_workspace FROM source_schema_attempt WHERE id=NEW.source_schema_attempt_id;\n  IF event_workspace IS DISTINCT FROM subject_workspace OR NOT EXISTS(SELECT 1 FROM cost_event WHERE id=NEW.cost_event_id AND activity_id=NEW.source_schema_attempt_id AND cost_type=''SOURCE_SCHEMA_SCAN_INVOCATION'' AND quantity=1 AND unit=''invocation'' AND pricing_mode=''ACTUAL'') THEN RAISE EXCEPTION ''invalid source schema cost allocation''; END IF;\n  RETURN NEW;\n END IF;\n-- source-schema guard end\n';
 IF position(E'\nBEGIN\n' IN definition)=0 THEN RAISE EXCEPTION 'unsupported cost workspace contract'; END IF;
 EXECUTE replace(definition,E'\nBEGIN\n',E'\nBEGIN\n'||guard);
END; $$;
