-- Closed fixture reference demands retain business FK history. A release is
-- append-only, exact-copy scoped, and never inferred from status/lease time.
CREATE TABLE lifecycle_fixture_reference_requirement (
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 copy_id uuid NOT NULL,
 dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
 kind varchar(32) NOT NULL,
 lineage_id uuid REFERENCES dataset_version_lineage(id),
 entity_match_job_id uuid REFERENCES entity_match_job(id),
 execution_id uuid ,
 execution_mapping_usage_id uuid REFERENCES execution_mapping_usage(id),
 quality_result_id uuid REFERENCES quality_result(id),
 quality_assessment_attempt_id uuid REFERENCES quality_assessment_attempt(id),
 dataset_certification_id uuid REFERENCES dataset_certification(id),
 delivery_operation_id uuid REFERENCES delivery_operation(id),
 input_name varchar(128),
 basis_ref varchar(255) NOT NULL CHECK (basis_ref<>'' AND basis_ref=btrim(basis_ref)),
 actor_id uuid NOT NULL,
 principal_ref varchar(255) NOT NULL CHECK (principal_ref<>''),
 dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 occurred_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
 FOREIGN KEY(execution_id,input_name) REFERENCES execution_input(execution_id,input_name),
 UNIQUE(workspace_id,id),
 CHECK(num_nonnulls(lineage_id,entity_match_job_id,execution_id,execution_mapping_usage_id,quality_result_id,quality_assessment_attempt_id,dataset_certification_id,delivery_operation_id)=1),
 CHECK((kind='LINEAGE' AND lineage_id IS NOT NULL AND input_name IS NULL) OR (kind='ENTITY_MATCH' AND entity_match_job_id IS NOT NULL AND input_name IS NULL) OR (kind='EXECUTION_INPUT' AND execution_id IS NOT NULL AND input_name IS NOT NULL) OR (kind='EXECUTION_MAPPING' AND execution_mapping_usage_id IS NOT NULL AND input_name IS NULL) OR (kind='QUALITY_RESULT' AND quality_result_id IS NOT NULL AND input_name IS NULL) OR (kind='QUALITY_ATTEMPT' AND quality_assessment_attempt_id IS NOT NULL AND input_name IS NULL) OR (kind='CERTIFICATION' AND dataset_certification_id IS NOT NULL AND input_name IS NULL) OR (kind='DELIVERY' AND delivery_operation_id IS NOT NULL AND input_name IS NULL)),
 UNIQUE(copy_id,lineage_id),
 UNIQUE(copy_id,entity_match_job_id),
 UNIQUE(copy_id,execution_id,input_name),
 UNIQUE(copy_id,execution_mapping_usage_id),
 UNIQUE(copy_id,quality_result_id),
 UNIQUE(copy_id,quality_assessment_attempt_id),
 UNIQUE(copy_id,dataset_certification_id),
 UNIQUE(copy_id,delivery_operation_id)
);
CREATE TABLE lifecycle_fixture_reference_release (
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 requirement_id uuid NOT NULL UNIQUE,
 loss varchar(96) NOT NULL,
 basis_ref varchar(255) NOT NULL CHECK(basis_ref<>'' AND basis_ref=btrim(basis_ref)),
 actor_id uuid NOT NULL,
 principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 occurred_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,requirement_id) REFERENCES lifecycle_fixture_reference_requirement(workspace_id,id)
);
CREATE FUNCTION lifecycle_reference_lock(w uuid, v uuid) RETURNS void AS $$
BEGIN
 INSERT INTO delivery_authorization_fence(workspace_id) VALUES(w) ON CONFLICT DO NOTHING;
 PERFORM 1 FROM delivery_authorization_fence WHERE workspace_id=w FOR UPDATE;
 PERFORM 1 FROM lifecycle_fixture_copy WHERE workspace_id=w AND dataset_version_id=v ORDER BY id FOR UPDATE;
END; $$ LANGUAGE plpgsql;

CREATE FUNCTION lifecycle_reference_fact_guard() RETURNS trigger AS $$
DECLARE r lifecycle_fixture_reference_requirement; valid boolean; expected_loss text;
BEGIN
 IF TG_TABLE_NAME='lifecycle_fixture_reference_requirement' THEN
  r:=NEW;
 ELSE
  SELECT * INTO STRICT r FROM lifecycle_fixture_reference_requirement WHERE id=NEW.requirement_id AND workspace_id=NEW.workspace_id;
 END IF;
 PERFORM lifecycle_reference_lock(r.workspace_id,r.dataset_version_id);
 IF NOT EXISTS(SELECT 1 FROM lifecycle_fixture_copy c WHERE c.id=r.copy_id AND c.workspace_id=r.workspace_id AND c.dataset_version_id=r.dataset_version_id) THEN
  RAISE EXCEPTION 'reference demand copy/workspace mismatch';
 END IF;
 IF TG_TABLE_NAME='lifecycle_fixture_reference_requirement' THEN
  CASE r.kind
   WHEN 'LINEAGE' THEN SELECT EXISTS(SELECT 1 FROM dataset_version_lineage s JOIN dataset_version ov ON ov.id=s.output_version_id JOIN dataset od ON od.id=ov.dataset_id WHERE s.id=r.lineage_id AND s.input_version_id=r.dataset_version_id AND od.workspace_id = r.workspace_id) INTO valid;
   WHEN 'ENTITY_MATCH' THEN SELECT EXISTS(SELECT 1 FROM entity_match_job s  WHERE s.id=r.entity_match_job_id AND s.input_dataset_version_id=r.dataset_version_id AND s.workspace_id = r.workspace_id) INTO valid;
   WHEN 'EXECUTION_INPUT' THEN SELECT EXISTS(SELECT 1 FROM execution_input s JOIN execution e ON e.id=s.execution_id WHERE s.execution_id=r.execution_id AND s.dataset_version_id=r.dataset_version_id AND e.workspace_id = r.workspace_id AND s.input_name=r.input_name) INTO valid;
   WHEN 'EXECUTION_MAPPING' THEN SELECT EXISTS(SELECT 1 FROM execution_mapping_usage s  WHERE s.id=r.execution_mapping_usage_id AND s.input_dataset_version_id=r.dataset_version_id AND s.workspace_id = r.workspace_id) INTO valid;
   WHEN 'QUALITY_RESULT' THEN SELECT EXISTS(SELECT 1 FROM quality_result s  WHERE s.id=r.quality_result_id AND s.dataset_version_id=r.dataset_version_id AND s.workspace_id = r.workspace_id) INTO valid;
   WHEN 'QUALITY_ATTEMPT' THEN SELECT EXISTS(SELECT 1 FROM quality_assessment_attempt s  WHERE s.id=r.quality_assessment_attempt_id AND s.dataset_version_id=r.dataset_version_id AND s.workspace_id = r.workspace_id) INTO valid;
   WHEN 'CERTIFICATION' THEN SELECT EXISTS(SELECT 1 FROM dataset_certification s  WHERE s.id=r.dataset_certification_id AND s.dataset_version_id=r.dataset_version_id AND s.workspace_id = r.workspace_id) INTO valid;
   WHEN 'DELIVERY' THEN SELECT EXISTS(SELECT 1 FROM delivery_operation s  WHERE s.id=r.delivery_operation_id AND s.dataset_version_id=r.dataset_version_id AND s.workspace_id = r.workspace_id) INTO valid;
   ELSE valid:=false;
  END CASE;
  IF NOT valid THEN RAISE EXCEPTION 'reference demand original identity/workspace mismatch'; END IF;
  IF EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d WHERE d.copy_id=r.copy_id AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release x WHERE x.dispatch_id=d.id)) THEN
   RAISE EXCEPTION 'reference demand after disposition dispatch';
  END IF;
 ELSE
  expected_loss:=CASE r.kind WHEN 'LINEAGE' THEN 'SOURCE_REPLAY_AND_CONTENT_REVERIFICATION_UNAVAILABLE' WHEN 'ENTITY_MATCH' THEN 'ENTITY_MATCH_REPLAY_UNAVAILABLE' WHEN 'EXECUTION_INPUT' THEN 'EXECUTION_REPLAY_UNAVAILABLE' WHEN 'EXECUTION_MAPPING' THEN 'MAPPING_INPUT_REVERIFICATION_UNAVAILABLE' WHEN 'QUALITY_RESULT' THEN 'QUALITY_RECOMPUTATION_UNAVAILABLE' WHEN 'QUALITY_ATTEMPT' THEN 'QUALITY_ATTEMPT_REPLAY_UNAVAILABLE' WHEN 'CERTIFICATION' THEN 'CERTIFICATION_CONTENT_REVERIFICATION_UNAVAILABLE' WHEN 'DELIVERY' THEN 'DELIVERY_PAYLOAD_REPLAY_UNAVAILABLE' END;
  IF NEW.loss IS DISTINCT FROM expected_loss OR NEW.occurred_at<r.occurred_at THEN RAISE EXCEPTION 'reference release loss/time mismatch'; END IF;
  IF NOT EXISTS(SELECT 1 FROM lifecycle_content_use_restriction WHERE workspace_id=r.workspace_id AND dataset_version_id=r.dataset_version_id) THEN RAISE EXCEPTION 'reference release requires stop-new-use'; END IF;
  IF EXISTS(SELECT 1 FROM lifecycle_reader_pin p WHERE p.workspace_id=r.workspace_id AND p.copy_id IN(SELECT id FROM lifecycle_fixture_copy WHERE workspace_id=r.workspace_id AND dataset_version_id=r.dataset_version_id) AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)) THEN RAISE EXCEPTION 'reference release requires verified owner stop'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_reference_requirement_scope BEFORE INSERT ON lifecycle_fixture_reference_requirement FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_fact_guard();
CREATE TRIGGER lifecycle_reference_release_scope BEFORE INSERT ON lifecycle_fixture_reference_release FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_fact_guard();
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['lifecycle_fixture_reference_requirement','lifecycle_fixture_reference_release'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()', n||'_immutable',n);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()', n||'_no_truncate',n);
 END LOOP;
END; $$;

-- Apply admission serialization to every current non-lifecycle FK, including
-- unmanaged domains. Future FK tables without this trigger block inspection.
CREATE FUNCTION lifecycle_reference_admission_guard() RETURNS trigger AS $$
DECLARE v uuid; w uuid;
BEGIN
 v:=(to_jsonb(NEW)->>TG_ARGV[0])::uuid;
 IF v IS NULL OR (TG_OP='UPDATE' AND to_jsonb(OLD)->>TG_ARGV[0]=v::text) THEN RETURN NEW; END IF;
 SELECT d.workspace_id INTO w FROM dataset_version dv JOIN dataset d ON d.id=dv.dataset_id WHERE dv.id=v;
 IF w IS NULL OR NOT EXISTS(SELECT 1 FROM lifecycle_fixture_copy WHERE dataset_version_id=v) THEN RETURN NEW; END IF;
 PERFORM lifecycle_reference_lock(w,v);
 IF EXISTS(SELECT 1 FROM lifecycle_content_use_restriction WHERE workspace_id=w AND dataset_version_id=v)
 OR EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d JOIN lifecycle_fixture_copy c ON c.id=d.copy_id WHERE c.dataset_version_id=v AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release x WHERE x.dispatch_id=d.id)) THEN
  RAISE EXCEPTION 'new fixture reference after restriction or disposition';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT DISTINCT ns.nspname,t.relname,a.attname FROM pg_constraint f JOIN pg_class t ON t.oid=f.conrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN LATERAL unnest(f.conkey,f.confkey) AS k(local_col,foreign_col) ON true JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.local_col JOIN pg_attribute b ON b.attrelid=f.confrelid AND b.attnum=k.foreign_col WHERE f.contype='f' AND f.confrelid='dataset_version'::regclass AND b.attname='id' AND t.relname NOT IN ('lifecycle_fixture_copy','lifecycle_content_use_restriction','lifecycle_copy_policy','lifecycle_restore_checkpoint','lifecycle_fixture_reference_requirement')
 LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE INSERT OR UPDATE OF %I ON %I.%I FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard(%L)', 'aa_lifecycle_ref_'||r.attname,r.attname,r.nspname,r.relname,r.attname);
 END LOOP;
END; $$;

-- A pending copy/registered late target cannot cross the dispatch fence.
CREATE FUNCTION lifecycle_pending_copy_admission_guard() RETURNS trigger AS $$
DECLARE v uuid; w uuid;
BEGIN
 w:=NEW.workspace_id;
 IF TG_TABLE_NAME='lifecycle_copy_operation' THEN
  SELECT dataset_version_id INTO STRICT v FROM lifecycle_fixture_copy WHERE id=NEW.source_copy_id AND workspace_id=w;
 ELSE v:=NEW.dataset_version_id;
 END IF;
 PERFORM lifecycle_reference_lock(w,v);
 IF EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d JOIN lifecycle_fixture_copy c ON c.id=d.copy_id WHERE c.workspace_id=w AND c.dataset_version_id=v AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release x WHERE x.dispatch_id=d.id)) THEN
  RAISE EXCEPTION 'pending copy after disposition dispatch';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER aa_lifecycle_pending_copy BEFORE INSERT ON lifecycle_copy_operation FOR EACH ROW EXECUTE FUNCTION lifecycle_pending_copy_admission_guard();
CREATE TRIGGER aa_lifecycle_registered_copy BEFORE INSERT ON lifecycle_fixture_copy FOR EACH ROW EXECUTE FUNCTION lifecycle_pending_copy_admission_guard();

