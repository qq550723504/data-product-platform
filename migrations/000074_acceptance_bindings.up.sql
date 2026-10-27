-- Core business responsibilities and applicability; no IAM or write HTTP route.
CREATE TABLE acceptance_party_representation (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, actor_id uuid NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'),
 principal_subject text NOT NULL CHECK(length(btrim(principal_subject)) BETWEEN 1 AND 255),
 party_ref text NOT NULL CHECK(length(btrim(party_ref)) BETWEEN 1 AND 255),
 basis_evidence_id uuid NOT NULL REFERENCES evidence(id), valid_from timestamptz NOT NULL, valid_to timestamptz,
 origin text NOT NULL CHECK(origin IN ('CORE_CONFIRMED','SYNTHETIC_FIXTURE')), recorded_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), CHECK(valid_to IS NULL OR valid_to>valid_from), UNIQUE(id,workspace_id)
);
CREATE TABLE acceptance_responsibility_binding (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, delivery_operation_id uuid NOT NULL REFERENCES delivery_operation(id),
 dataset_version_id uuid NOT NULL REFERENCES dataset_version(id), role text NOT NULL CHECK(role IN ('PROVIDER','RECEIVER','BASIS_CONFIRMER','REVIEWER','ARCHIVIST','DISPOSER')),
 representation_id uuid NOT NULL, basis_evidence_id uuid NOT NULL REFERENCES evidence(id), valid_from timestamptz NOT NULL, valid_to timestamptz,
 origin text NOT NULL CHECK(origin IN ('CORE_CONFIRMED','SYNTHETIC_FIXTURE')), recorded_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(representation_id,workspace_id) REFERENCES acceptance_party_representation(id,workspace_id),
 CHECK(valid_to IS NULL OR valid_to>valid_from), UNIQUE(id,workspace_id)
);
CREATE INDEX acceptance_responsibility_scope ON acceptance_responsibility_binding(workspace_id,delivery_operation_id,dataset_version_id,role);
CREATE TABLE acceptance_standard_applicability (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, delivery_operation_id uuid NOT NULL REFERENCES delivery_operation(id),
 dataset_version_id uuid NOT NULL REFERENCES dataset_version(id), contract_version_id uuid NOT NULL REFERENCES contract_version(id),
 source_sha256 text NOT NULL CHECK(source_sha256 ~ '^[0-9a-f]{64}$'), criteria_set_hash text NOT NULL CHECK(criteria_set_hash ~ '^[0-9a-f]{64}$'),
 confirmer_binding_id uuid NOT NULL, basis_evidence_id uuid NOT NULL REFERENCES evidence(id), valid_from timestamptz NOT NULL, valid_to timestamptz,
 origin text NOT NULL CHECK(origin IN ('CORE_CONFIRMED','SYNTHETIC_FIXTURE')), recorded_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(confirmer_binding_id,workspace_id) REFERENCES acceptance_responsibility_binding(id,workspace_id),
 CHECK(valid_to IS NULL OR valid_to>valid_from), UNIQUE(id,workspace_id)
);
CREATE INDEX acceptance_applicability_scope ON acceptance_standard_applicability(workspace_id,delivery_operation_id,dataset_version_id,contract_version_id);
CREATE TABLE acceptance_binding_disposition (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, representation_id uuid, responsibility_id uuid, applicability_id uuid,
 disposition text NOT NULL CHECK(disposition IN ('INVALIDATED','REVOKED','SUPERSEDED')), effective_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 1000), basis_evidence_id uuid NOT NULL REFERENCES evidence(id),
 recorded_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), CHECK(num_nonnulls(representation_id,responsibility_id,applicability_id)=1),
 FOREIGN KEY(representation_id,workspace_id) REFERENCES acceptance_party_representation(id,workspace_id),
 FOREIGN KEY(responsibility_id,workspace_id) REFERENCES acceptance_responsibility_binding(id,workspace_id),
 FOREIGN KEY(applicability_id,workspace_id) REFERENCES acceptance_standard_applicability(id,workspace_id)
);
CREATE INDEX acceptance_disposition_representation ON acceptance_binding_disposition(workspace_id,representation_id) WHERE representation_id IS NOT NULL;
CREATE INDEX acceptance_disposition_responsibility ON acceptance_binding_disposition(workspace_id,responsibility_id) WHERE responsibility_id IS NOT NULL;
CREATE INDEX acceptance_disposition_applicability ON acceptance_binding_disposition(workspace_id,applicability_id) WHERE applicability_id IS NOT NULL;
CREATE FUNCTION guard_acceptance_binding_fact() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ws uuid; dv uuid; expected_consumer text; party text; role_value text; source_hash text;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'acceptance binding fact is immutable' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.basis_evidence_id AND workspace_id=NEW.workspace_id) THEN RAISE EXCEPTION 'acceptance evidence crosses workspace' USING ERRCODE='23514'; END IF;
 IF TG_TABLE_NAME IN ('acceptance_responsibility_binding','acceptance_standard_applicability') THEN
  SELECT o.workspace_id,o.dataset_version_id,o.effective_consumer_ref INTO ws,dv,expected_consumer FROM delivery_operation o WHERE o.id=NEW.delivery_operation_id;
  IF ws IS DISTINCT FROM NEW.workspace_id OR dv IS DISTINCT FROM NEW.dataset_version_id OR NOT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=dv AND d.workspace_id=ws) THEN RAISE EXCEPTION 'acceptance target crosses scope' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='acceptance_responsibility_binding' THEN
   SELECT party_ref INTO party FROM acceptance_party_representation WHERE id=NEW.representation_id AND workspace_id=ws;
   IF party IS NULL OR (NEW.role IN ('RECEIVER','BASIS_CONFIRMER','REVIEWER') AND party<>expected_consumer) THEN RAISE EXCEPTION 'acceptance representative does not match receiving party' USING ERRCODE='23514'; END IF;
  ELSE
   SELECT c.workspace_id,v.source_sha256 INTO ws,source_hash FROM contract_version v JOIN data_contract c ON c.id=v.contract_id WHERE v.id=NEW.contract_version_id;
   IF ws IS DISTINCT FROM NEW.workspace_id OR source_hash IS DISTINCT FROM NEW.source_sha256 THEN RAISE EXCEPTION 'acceptance standard source crosses scope/hash' USING ERRCODE='23514'; END IF;
   SELECT role INTO role_value FROM acceptance_responsibility_binding WHERE id=NEW.confirmer_binding_id AND workspace_id=ws AND delivery_operation_id=NEW.delivery_operation_id AND dataset_version_id=NEW.dataset_version_id;
   IF role_value IS DISTINCT FROM 'BASIS_CONFIRMER' THEN RAISE EXCEPTION 'acceptance confirmer does not match target/role' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
REVOKE EXECUTE ON FUNCTION guard_acceptance_binding_fact() FROM PUBLIC;
CREATE TRIGGER acceptance_representation_guard BEFORE INSERT OR UPDATE OR DELETE ON acceptance_party_representation FOR EACH ROW EXECUTE FUNCTION guard_acceptance_binding_fact();
CREATE TRIGGER acceptance_responsibility_guard BEFORE INSERT OR UPDATE OR DELETE ON acceptance_responsibility_binding FOR EACH ROW EXECUTE FUNCTION guard_acceptance_binding_fact();
CREATE TRIGGER acceptance_applicability_guard BEFORE INSERT OR UPDATE OR DELETE ON acceptance_standard_applicability FOR EACH ROW EXECUTE FUNCTION guard_acceptance_binding_fact();
CREATE TRIGGER acceptance_disposition_guard BEFORE INSERT OR UPDATE OR DELETE ON acceptance_binding_disposition FOR EACH ROW EXECUTE FUNCTION guard_acceptance_binding_fact();
CREATE TRIGGER aa_lifecycle_ref_dataset_version_id BEFORE INSERT OR UPDATE OF dataset_version_id ON acceptance_responsibility_binding FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard('dataset_version_id');
CREATE TRIGGER aa_lifecycle_ref_dataset_version_id BEFORE INSERT OR UPDATE OF dataset_version_id ON acceptance_standard_applicability FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard('dataset_version_id');
