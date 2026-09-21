-- #134 DatasetCertification and append-only CertificationDisposition.
-- This migration is intentionally ordered after #137: certification references
-- finalized RightsSnapshot and EffectiveRightsSnapshot facts, never a temporary
-- rights calculation.
CREATE TABLE dataset_certification (
    id                              uuid PRIMARY KEY,
    workspace_id                    uuid NOT NULL,
    dataset_version_id              uuid NOT NULL REFERENCES dataset_version(id),
    quality_assessment_id           uuid NOT NULL REFERENCES quality_result(id),
    certification_profile_id        uuid NOT NULL REFERENCES certification_profile(id),
    profile_ref                     varchar(512) NOT NULL,
    profile_version                 varchar(64) NOT NULL,
    profile_content_sha256          varchar(64) NOT NULL,
    profile_content_snapshot        text NOT NULL,
    rights_snapshot_id              uuid REFERENCES rights_snapshot(id),
    effective_rights_snapshot_id    uuid REFERENCES effective_rights_snapshot(id),
    effective_rights_snapshot_hash  varchar(64),
    frozen_rights_context_hash      varchar(64),
    compliance_result_id            uuid REFERENCES compliance_result(id),
    contract_version_id             uuid REFERENCES contract_version(id),
    traceability_evidence_id        uuid,
    evidence_snapshot_id            uuid REFERENCES evidence_snapshot(id),
    decision                        varchar(16) NOT NULL,
    blockers                        jsonb NOT NULL DEFAULT '[]'::jsonb,
    reason                          varchar(2048) NOT NULL,
    issued_at                       timestamptz NOT NULL,
    created_by                      uuid,
    CONSTRAINT ck_dataset_certification_profile_hash CHECK (profile_content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_dataset_certification_profile_hash_matches CHECK (
        encode(digest(convert_to(profile_content_snapshot, 'UTF8'), 'sha256'), 'hex') = profile_content_sha256
    ),
    CONSTRAINT ck_dataset_certification_decision CHECK (decision IN ('CERTIFIED','REJECTED')),
    CONSTRAINT ck_dataset_certification_blockers_array CHECK (jsonb_typeof(blockers) = 'array')
);

CREATE INDEX idx_dataset_certification_target
    ON dataset_certification(workspace_id, dataset_version_id, certification_profile_id, issued_at, id);
CREATE INDEX idx_dataset_certification_quality
    ON dataset_certification(quality_assessment_id);

CREATE TABLE certification_disposition (
    id                              uuid PRIMARY KEY,
    workspace_id                    uuid NOT NULL,
    certification_id                uuid NOT NULL REFERENCES dataset_certification(id),
    disposition                     varchar(16) NOT NULL,
    effective_at                    timestamptz NOT NULL,
    reason                          varchar(2048) NOT NULL,
    superseded_by_certification_id  uuid REFERENCES dataset_certification(id),
    evidence_snapshot_id            uuid REFERENCES evidence_snapshot(id),
    created_at                      timestamptz NOT NULL DEFAULT now(),
    created_by                      uuid,
    CONSTRAINT uq_certification_disposition_kind UNIQUE(certification_id, disposition),
    CONSTRAINT ck_certification_disposition_kind CHECK (disposition IN ('REVOKED','SUPERSEDED')),
    CONSTRAINT ck_certification_disposition_replacement CHECK (
        disposition <> 'SUPERSEDED'
        OR (superseded_by_certification_id IS NOT NULL AND superseded_by_certification_id <> certification_id)
    )
);

CREATE INDEX idx_certification_disposition_current
    ON certification_disposition(certification_id, effective_at, id);

CREATE OR REPLACE FUNCTION validate_dataset_certification_references()
RETURNS trigger AS $$
DECLARE
    dataset_workspace uuid;
    quality_workspace uuid;
    quality_dataset_version uuid;
    profile_workspace uuid;
    profile_ref_value varchar(512);
    profile_version_value varchar(64);
    profile_hash_value varchar(64);
    profile_content_value text;
    profile_rights_required boolean;
    profile_compliance_required boolean;
    profile_contract_required boolean;
    profile_evidence_required boolean;
    rights_workspace uuid;
    rights_status varchar(16);
    effective_workspace uuid;
    effective_dataset_version uuid;
    effective_status varchar(16);
    effective_root_hash varchar(64);
    compliance_workspace uuid;
    compliance_dataset_version uuid;
    contract_workspace uuid;
    evidence_workspace uuid;
BEGIN
    SELECT d.workspace_id INTO dataset_workspace
      FROM dataset_version v JOIN dataset d ON d.id = v.dataset_id
     WHERE v.id = NEW.dataset_version_id;
    IF dataset_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'DatasetCertification crosses workspace boundary';
    END IF;

    SELECT workspace_id, dataset_version_id INTO quality_workspace, quality_dataset_version
      FROM quality_result WHERE id = NEW.quality_assessment_id;
    IF quality_workspace IS DISTINCT FROM NEW.workspace_id OR quality_dataset_version IS DISTINCT FROM NEW.dataset_version_id THEN
        RAISE EXCEPTION 'DatasetCertification quality assessment does not match target';
    END IF;

    SELECT workspace_id, profile_ref, version, content_sha256, content_snapshot,
           rights_required, compliance_required, contract_required, evidence_required
      INTO profile_workspace, profile_ref_value, profile_version_value, profile_hash_value, profile_content_value,
           profile_rights_required, profile_compliance_required, profile_contract_required, profile_evidence_required
      FROM certification_profile WHERE id = NEW.certification_profile_id;
    IF profile_workspace IS DISTINCT FROM NEW.workspace_id
       OR profile_ref_value IS DISTINCT FROM NEW.profile_ref
       OR profile_version_value IS DISTINCT FROM NEW.profile_version
       OR profile_hash_value IS DISTINCT FROM NEW.profile_content_sha256
       OR profile_content_value IS DISTINCT FROM NEW.profile_content_snapshot THEN
        RAISE EXCEPTION 'DatasetCertification profile snapshot does not match immutable profile';
    END IF;

    IF NEW.decision = 'CERTIFIED' AND profile_rights_required AND (
        NEW.rights_snapshot_id IS NULL OR
        NEW.effective_rights_snapshot_id IS NULL OR
        NEW.effective_rights_snapshot_hash IS NULL OR
        NEW.frozen_rights_context_hash IS NULL
    ) THEN
        RAISE EXCEPTION 'DatasetCertification profile requires frozen rights evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_compliance_required AND NEW.compliance_result_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires compliance evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_contract_required AND NEW.contract_version_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires contract evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_evidence_required AND NEW.evidence_snapshot_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires evidence snapshot';
    END IF;

    IF NEW.rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id, status INTO rights_workspace, rights_status
          FROM rights_snapshot WHERE id = NEW.rights_snapshot_id;
        IF rights_workspace IS DISTINCT FROM NEW.workspace_id OR rights_status <> 'FINALIZED' THEN
            RAISE EXCEPTION 'DatasetCertification requires a finalized same-workspace RightsSnapshot';
        END IF;
    END IF;

    IF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id, target_dataset_version_id, status, root_hash
          INTO effective_workspace, effective_dataset_version, effective_status, effective_root_hash
          FROM effective_rights_snapshot WHERE id = NEW.effective_rights_snapshot_id;
        IF effective_workspace IS DISTINCT FROM NEW.workspace_id
           OR effective_dataset_version IS DISTINCT FROM NEW.dataset_version_id
           OR effective_status <> 'FINALIZED'
           OR effective_root_hash IS DISTINCT FROM NEW.effective_rights_snapshot_hash THEN
            RAISE EXCEPTION 'DatasetCertification requires a finalized matching EffectiveRightsSnapshot';
        END IF;
    END IF;

    IF NEW.compliance_result_id IS NOT NULL THEN
        SELECT workspace_id, dataset_version_id INTO compliance_workspace, compliance_dataset_version
          FROM compliance_result WHERE id = NEW.compliance_result_id;
        IF compliance_workspace IS DISTINCT FROM NEW.workspace_id OR compliance_dataset_version IS DISTINCT FROM NEW.dataset_version_id THEN
            RAISE EXCEPTION 'DatasetCertification compliance result does not match target';
        END IF;
    END IF;

    IF NEW.contract_version_id IS NOT NULL THEN
        SELECT c.workspace_id INTO contract_workspace
          FROM contract_version cv JOIN data_contract c ON c.id = cv.contract_id
         WHERE cv.id = NEW.contract_version_id;
        IF contract_workspace IS DISTINCT FROM NEW.workspace_id THEN
            RAISE EXCEPTION 'DatasetCertification contract does not match workspace';
        END IF;
    END IF;

    IF NEW.evidence_snapshot_id IS NOT NULL THEN
        SELECT workspace_id INTO evidence_workspace
          FROM evidence_snapshot WHERE id = NEW.evidence_snapshot_id;
        IF evidence_workspace IS DISTINCT FROM NEW.workspace_id THEN
            RAISE EXCEPTION 'DatasetCertification evidence does not match workspace';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_certification_references
BEFORE INSERT ON dataset_certification
FOR EACH ROW EXECUTE FUNCTION validate_dataset_certification_references();

CREATE OR REPLACE FUNCTION prevent_dataset_certification_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'DatasetCertification is an immutable historical fact';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_certification_immutable
BEFORE UPDATE OR DELETE ON dataset_certification
FOR EACH ROW EXECUTE FUNCTION prevent_dataset_certification_mutation();

CREATE OR REPLACE FUNCTION validate_certification_disposition_references()
RETURNS trigger AS $$
DECLARE certification_workspace uuid;
BEGIN
    SELECT workspace_id INTO certification_workspace
      FROM dataset_certification WHERE id = NEW.certification_id;
    IF certification_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'CertificationDisposition crosses workspace boundary';
    END IF;
    IF NEW.superseded_by_certification_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM dataset_certification c
             WHERE c.id = NEW.superseded_by_certification_id
               AND c.workspace_id = NEW.workspace_id
        ) THEN
            RAISE EXCEPTION 'CertificationDisposition replacement is not in the same workspace';
        END IF;
    END IF;
    IF NEW.evidence_snapshot_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM evidence_snapshot e WHERE e.id = NEW.evidence_snapshot_id AND e.workspace_id = NEW.workspace_id
    ) THEN
        RAISE EXCEPTION 'CertificationDisposition evidence is not in the same workspace';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_certification_disposition_references
BEFORE INSERT ON certification_disposition
FOR EACH ROW EXECUTE FUNCTION validate_certification_disposition_references();

CREATE OR REPLACE FUNCTION prevent_certification_disposition_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'CertificationDisposition is append-only history';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_certification_disposition_immutable
BEFORE UPDATE OR DELETE ON certification_disposition
FOR EACH ROW EXECUTE FUNCTION prevent_certification_disposition_mutation();

-- Certification and disposition activities are typed cost subjects. The
-- workspace guard installed by #137 is replaced with the same guard plus the
-- two certification subjects.
ALTER TABLE cost_allocation
    ADD COLUMN dataset_certification_id uuid REFERENCES dataset_certification(id),
    ADD COLUMN certification_disposition_id uuid REFERENCES certification_disposition(id);

ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
    (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_attempt_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_verification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN effective_rights_snapshot_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN dataset_certification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN certification_disposition_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE INDEX idx_cost_allocation_dataset_certification
    ON cost_allocation(dataset_certification_id) WHERE dataset_certification_id IS NOT NULL;
CREATE INDEX idx_cost_allocation_certification_disposition
    ON cost_allocation(certification_disposition_id) WHERE certification_disposition_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace()
RETURNS trigger AS $$
DECLARE
    event_workspace uuid;
    subject_workspace uuid;
BEGIN
    SELECT workspace_id INTO event_workspace FROM cost_event WHERE id = NEW.cost_event_id;
    IF NEW.delivery_operation_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM delivery_operation WHERE id = NEW.delivery_operation_id;
    ELSIF NEW.quality_assessment_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_result WHERE id = NEW.quality_assessment_id;
    ELSIF NEW.quality_assessment_attempt_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_assessment_attempt WHERE id = NEW.quality_assessment_attempt_id;
    ELSIF NEW.rights_declaration_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM rights_declaration WHERE id = NEW.rights_declaration_id;
    ELSIF NEW.rights_declaration_verification_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace FROM rights_declaration_verification v JOIN rights_declaration d ON d.id=v.declaration_id WHERE v.id=NEW.rights_declaration_verification_id;
    ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace FROM rights_declaration_disposition x JOIN rights_declaration d ON d.id=x.declaration_id WHERE x.id=NEW.rights_declaration_disposition_id;
    ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM authorization_provenance_binding WHERE id=NEW.authorization_provenance_binding_id;
    ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
        SELECT b.workspace_id INTO subject_workspace FROM authorization_provenance_binding_disposition x JOIN authorization_provenance_binding b ON b.id=x.binding_id WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
    ELSIF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM effective_rights_snapshot WHERE id=NEW.effective_rights_snapshot_id;
    ELSIF NEW.dataset_certification_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM dataset_certification WHERE id=NEW.dataset_certification_id;
    ELSE
        SELECT workspace_id INTO subject_workspace FROM certification_disposition WHERE id=NEW.certification_disposition_id;
    END IF;
    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
