-- #207 Gold Certification proof binding.
-- Extends the existing immutable DatasetCertification fact; no parallel Gold
-- certification aggregate is introduced.

ALTER TABLE dataset_certification
    ADD COLUMN gold_production_binding_id uuid REFERENCES gold_production_binding(id),
    ADD COLUMN annotation_snapshot_id uuid REFERENCES annotation_snapshot(id),
    ADD COLUMN annotation_snapshot_root_hash varchar(64),
    ADD COLUMN annotation_schema_content_sha256 varchar(64),
    ADD COLUMN annotation_taxonomy_content_sha256 varchar(64),
    ADD COLUMN gold_production_binding_root_hash varchar(64),
    ADD CONSTRAINT ck_dataset_certification_gold_proof CHECK (
        (
            gold_production_binding_id IS NULL
            AND annotation_snapshot_id IS NULL
            AND annotation_snapshot_root_hash IS NULL
            AND annotation_schema_content_sha256 IS NULL
            AND annotation_taxonomy_content_sha256 IS NULL
            AND gold_production_binding_root_hash IS NULL
        )
        OR
        (
            gold_production_binding_id IS NOT NULL
            AND annotation_snapshot_id IS NOT NULL
            AND annotation_snapshot_root_hash ~ '^[0-9a-f]{64}$'
            AND annotation_schema_content_sha256 ~ '^[0-9a-f]{64}$'
            AND annotation_taxonomy_content_sha256 ~ '^[0-9a-f]{64}$'
            AND gold_production_binding_root_hash ~ '^[0-9a-f]{64}$'
        )
    );

CREATE INDEX idx_dataset_certification_gold_binding
    ON dataset_certification(gold_production_binding_id)
    WHERE gold_production_binding_id IS NOT NULL;

CREATE OR REPLACE FUNCTION validate_dataset_certification_references()
RETURNS trigger AS $$
DECLARE
    dataset_workspace uuid;
    dataset_status varchar(32);
    quality_workspace uuid;
    quality_dataset_version uuid;
    quality_gate varchar(32);
    quality_rule_set_ref varchar(512);
    quality_evaluator_name varchar(255);
    profile_workspace uuid;
    profile_ref_value varchar(512);
    profile_version_value varchar(64);
    profile_hash_value varchar(64);
    profile_content_value text;
    profile_quality_gate_required boolean;
    profile_rights_required boolean;
    profile_rights_purpose_mode varchar(16);
    profile_rights_action_mode varchar(16);
    profile_rights_consumer_mode varchar(16);
    profile_rights_scope_mode varchar(16);
    profile_compliance_required boolean;
    profile_contract_required boolean;
    profile_contract_code varchar(128);
    profile_traceability_required boolean;
    profile_evidence_required boolean;
    rights_workspace uuid;
    rights_status varchar(16);
    rights_purpose varchar(128);
    rights_consumer varchar(255);
    rights_referenced_by_effective boolean;
    effective_workspace uuid;
    effective_dataset_version uuid;
    effective_status varchar(16);
    effective_root_hash varchar(64);
    effective_consumer_ref varchar(255);
    effective_purpose varchar(128);
    effective_lineage_mismatch boolean;
    gold_binding_workspace uuid;
    gold_binding_output_version uuid;
    gold_binding_snapshot uuid;
    gold_binding_snapshot_root varchar(64);
    gold_binding_schema_hash varchar(64);
    gold_binding_taxonomy_hash varchar(64);
    gold_binding_root_hash varchar(64);
    gold_binding_status varchar(16);
    gold_snapshot_status varchar(16);
    gold_snapshot_root varchar(64);
    gold_campaign_schema_hash varchar(64);
    gold_campaign_taxonomy_hash varchar(64);
    compliance_workspace uuid;
    compliance_dataset_version uuid;
    compliance_gate varchar(32);
    contract_workspace uuid;
    contract_code_value varchar(128);
    contract_status varchar(16);
    traceability_workspace uuid;
    traceability_object_type varchar(64);
    traceability_object_id uuid;
    traceability_has_items boolean;
    evidence_workspace uuid;
    evidence_object_type varchar(64);
    evidence_object_id uuid;
BEGIN
    SELECT d.workspace_id, v.status INTO dataset_workspace, dataset_status
      FROM dataset_version v JOIN dataset d ON d.id = v.dataset_id
     WHERE v.id = NEW.dataset_version_id
     FOR SHARE OF v;
    IF dataset_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'DatasetCertification crosses workspace boundary';
    END IF;

    SELECT workspace_id, dataset_version_id, gate_decision, rule_set_ref, evaluator_name
      INTO quality_workspace, quality_dataset_version, quality_gate, quality_rule_set_ref, quality_evaluator_name
      FROM quality_result WHERE id = NEW.quality_assessment_id
      FOR SHARE;
    IF quality_workspace IS DISTINCT FROM NEW.workspace_id OR quality_dataset_version IS DISTINCT FROM NEW.dataset_version_id THEN
        RAISE EXCEPTION 'DatasetCertification quality assessment does not match target';
    END IF;

    SELECT workspace_id, profile_ref, version, content_sha256, content_snapshot,
           quality_gate_required,
           rights_required, rights_purpose_mode, rights_action_mode, rights_consumer_mode, rights_scope_mode,
           compliance_required, contract_required, contract_code, traceability_required, evidence_required
      INTO profile_workspace, profile_ref_value, profile_version_value, profile_hash_value, profile_content_value,
           profile_quality_gate_required,
           profile_rights_required, profile_rights_purpose_mode, profile_rights_action_mode, profile_rights_consumer_mode, profile_rights_scope_mode,
           profile_compliance_required, profile_contract_required, profile_contract_code, profile_traceability_required, profile_evidence_required
      FROM certification_profile WHERE id = NEW.certification_profile_id;
    IF profile_workspace IS DISTINCT FROM NEW.workspace_id
       OR profile_ref_value IS DISTINCT FROM NEW.profile_ref
       OR profile_version_value IS DISTINCT FROM NEW.profile_version
       OR profile_hash_value IS DISTINCT FROM NEW.profile_content_sha256
       OR profile_content_value IS DISTINCT FROM NEW.profile_content_snapshot THEN
        RAISE EXCEPTION 'DatasetCertification profile snapshot does not match immutable profile';
    END IF;

    IF NEW.decision = 'CERTIFIED' AND dataset_status <> 'READY' THEN
        RAISE EXCEPTION 'DatasetCertification requires a READY DatasetVersion';
    END IF;

    IF NEW.decision = 'CERTIFIED' AND profile_quality_gate_required AND quality_gate <> 'PASS' THEN
        RAISE EXCEPTION 'DatasetCertification requires a passing QualityAssessment gate';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND EXISTS (
        SELECT 1
        FROM certification_profile_quality_dimension d
        WHERE d.profile_id=NEW.certification_profile_id
          AND COALESCE((
              SELECT q.metrics->'dimensions'->d.dimension->>'status'
              FROM quality_result q WHERE q.id=NEW.quality_assessment_id
          ), '') <> 'PASS'
    ) THEN
        RAISE EXCEPTION 'DatasetCertification required quality dimension is not PASS';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND EXISTS (
        SELECT 1
        FROM certification_profile_critical_rule r
        WHERE r.profile_id=NEW.certification_profile_id
          AND NOT EXISTS (
              SELECT 1 FROM quality_finding f
              WHERE f.result_id=NEW.quality_assessment_id
                AND f.rule_id=r.rule_id
                AND f.status='PASS'
          )
    ) THEN
        RAISE EXCEPTION 'DatasetCertification required critical rule is not PASS';
    END IF;

    IF NEW.decision = 'CERTIFIED' AND profile_rights_required AND (
        NEW.effective_rights_snapshot_id IS NULL OR
        NEW.effective_rights_snapshot_hash IS NULL OR
        NEW.frozen_rights_context_hash IS NULL
    ) THEN
        RAISE EXCEPTION 'DatasetCertification profile requires frozen EffectiveRights evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_compliance_required AND NEW.compliance_result_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires compliance evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_contract_required AND NEW.contract_version_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires contract evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_traceability_required AND NEW.traceability_evidence_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires traceability evidence';
    END IF;
    IF NEW.decision = 'CERTIFIED' AND profile_evidence_required AND NEW.evidence_snapshot_id IS NULL THEN
        RAISE EXCEPTION 'DatasetCertification profile requires evidence snapshot';
    END IF;

    IF NEW.rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id, status, purpose, COALESCE(consumer_ref,'')
          INTO rights_workspace, rights_status, rights_purpose, rights_consumer
          FROM rights_snapshot WHERE id = NEW.rights_snapshot_id
          FOR SHARE;
        SELECT NOT EXISTS (
            SELECT 1
            FROM effective_rights_input eri
            WHERE eri.snapshot_id=NEW.effective_rights_snapshot_id
              AND eri.declaration_id IS NOT NULL
              AND NOT EXISTS (
                  SELECT 1 FROM rights_snapshot_declaration rsd
                  WHERE rsd.rights_snapshot_id=NEW.rights_snapshot_id
                    AND rsd.declaration_id=eri.declaration_id
              )
        ) AND NOT EXISTS (
            SELECT 1
            FROM effective_rights_input eri
            WHERE eri.snapshot_id=NEW.effective_rights_snapshot_id
              AND eri.binding_id IS NOT NULL
              AND NOT EXISTS (
                  SELECT 1 FROM rights_snapshot_provenance_binding rspb
                  WHERE rspb.rights_snapshot_id=NEW.rights_snapshot_id
                    AND rspb.binding_id=eri.binding_id
              )
        ) INTO rights_referenced_by_effective;
        IF rights_workspace IS DISTINCT FROM NEW.workspace_id
           OR rights_status <> 'FINALIZED'
           OR rights_purpose IS DISTINCT FROM (
               SELECT purpose FROM effective_rights_snapshot WHERE id=NEW.effective_rights_snapshot_id
           )
           OR rights_consumer IS DISTINCT FROM (
               SELECT COALESCE(consumer_ref,'') FROM effective_rights_snapshot WHERE id=NEW.effective_rights_snapshot_id
           )
           OR NOT rights_referenced_by_effective THEN
            RAISE EXCEPTION 'DatasetCertification RightsSnapshot does not cover frozen EffectiveRights provenance';
        END IF;
    END IF;

    IF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id, target_dataset_version_id, status, root_hash, consumer_ref, purpose
          INTO effective_workspace, effective_dataset_version, effective_status, effective_root_hash, effective_consumer_ref, effective_purpose
          FROM effective_rights_snapshot WHERE id = NEW.effective_rights_snapshot_id
          FOR SHARE;
        IF effective_workspace IS DISTINCT FROM NEW.workspace_id
           OR effective_dataset_version IS DISTINCT FROM NEW.dataset_version_id
           OR effective_status <> 'FINALIZED'
           OR effective_root_hash IS DISTINCT FROM NEW.effective_rights_snapshot_hash THEN
            RAISE EXCEPTION 'DatasetCertification requires a finalized matching EffectiveRightsSnapshot';
        END IF;

        IF NEW.decision = 'CERTIFIED' AND profile_rights_required THEN
            -- EffectiveRightsSnapshot is currently a single consumer/purpose context.
            -- It cannot prove universal coverage, so ANY requirements fail closed
            -- until the rights model has an explicit universal frozen fact.
            IF profile_rights_purpose_mode = 'ANY'
               OR profile_rights_action_mode = 'ANY'
               OR profile_rights_consumer_mode = 'ANY'
               OR profile_rights_scope_mode = 'ANY' THEN
                RAISE EXCEPTION 'DatasetCertification cannot prove ANY rights applicability from a single-context EffectiveRightsSnapshot';
            END IF;

            IF EXISTS (
                SELECT 1 FROM certification_profile_rights_purpose p
                WHERE p.profile_id=NEW.certification_profile_id
                  AND p.purpose_code IS DISTINCT FROM effective_purpose
            ) THEN
                RAISE EXCEPTION 'EffectiveRightsSnapshot purpose does not cover CertificationProfile';
            END IF;

            IF EXISTS (
                SELECT 1 FROM certification_profile_rights_consumer c
                WHERE c.profile_id=NEW.certification_profile_id
                  AND c.consumer_ref IS DISTINCT FROM effective_consumer_ref
            ) THEN
                RAISE EXCEPTION 'EffectiveRightsSnapshot consumer does not cover CertificationProfile';
            END IF;

            IF EXISTS (
                SELECT 1
                FROM certification_profile_rights_action a
                WHERE a.profile_id=NEW.certification_profile_id
                  AND NOT EXISTS (
                      SELECT 1 FROM effective_rights_action era
                      WHERE era.snapshot_id=NEW.effective_rights_snapshot_id
                        AND era.action=a.action
                        AND era.decision='ALLOWED'
                  )
            ) THEN
                RAISE EXCEPTION 'EffectiveRightsSnapshot actions do not cover CertificationProfile';
            END IF;

            IF EXISTS (
                SELECT 1
                FROM certification_profile_rights_scope s
                WHERE s.profile_id=NEW.certification_profile_id
                  AND (
                      s.scope_type <> 'ALL_RESOURCE'
                      OR NOT EXISTS (
                          SELECT 1 FROM effective_rights_input eri
                          WHERE eri.snapshot_id=NEW.effective_rights_snapshot_id
                            AND eri.data_resource_id::text=s.scope_ref
                      )
                  )
            ) THEN
                RAISE EXCEPTION 'EffectiveRightsSnapshot normalized scopes do not cover CertificationProfile';
            END IF;

            WITH RECURSIVE lineage(version_id) AS (
                SELECT NEW.dataset_version_id
                UNION
                SELECT edge.input_version_id
                FROM dataset_version_lineage edge
                JOIN lineage parent ON parent.version_id=edge.output_version_id
            ),
            dataset_dependencies AS (
                SELECT DISTINCT
                       'DATASET_VERSION'::text AS dependency_kind,
                       l.version_id AS dataset_version_id,
                       d.source_resource_id AS data_resource_id
                FROM lineage l
                JOIN dataset_version v ON v.id=l.version_id
                JOIN dataset d ON d.id=v.dataset_id
                WHERE NOT EXISTS (
                    SELECT 1 FROM dataset_version_lineage child
                    WHERE child.output_version_id=l.version_id
                )
            ),
            gold_resource_dependencies AS (
                SELECT DISTINCT
                       'ANNOTATION_CONTRIBUTION_RESOURCE'::text AS dependency_kind,
                       NULL::uuid AS dataset_version_id,
                       g.annotation_contribution_resource_id AS data_resource_id
                FROM lineage l
                JOIN gold_production_binding g
                  ON g.output_dataset_version_id=l.version_id
                 AND g.status='FINALIZED'
            ),
            required AS (
                SELECT * FROM dataset_dependencies
                UNION
                SELECT * FROM gold_resource_dependencies
            ),
            frozen AS (
                SELECT dependency_kind, input_dataset_version_id, data_resource_id
                FROM effective_rights_input
                WHERE snapshot_id=NEW.effective_rights_snapshot_id
            ),
            mismatch AS (
                (SELECT * FROM required EXCEPT SELECT * FROM frozen)
                UNION ALL
                (SELECT * FROM frozen EXCEPT SELECT * FROM required)
            )
            SELECT EXISTS(SELECT 1 FROM mismatch) INTO effective_lineage_mismatch;
            IF effective_lineage_mismatch THEN
                RAISE EXCEPTION 'EffectiveRightsSnapshot lineage membership does not match target DatasetVersion';
            END IF;
        END IF;
    END IF;

    IF profile_ref_value = 'gold/dataset-v1' THEN
        IF quality_rule_set_ref IS DISTINCT FROM 'gold/quality/annotation-v1'
           OR quality_evaluator_name IS DISTINCT FROM 'gold-quality' THEN
            RAISE EXCEPTION 'Gold DatasetCertification requires the formal Gold QualityAssessment';
        END IF;
        IF NEW.gold_production_binding_id IS NULL
           OR NEW.annotation_snapshot_id IS NULL
           OR NEW.annotation_snapshot_root_hash IS NULL
           OR NEW.annotation_schema_content_sha256 IS NULL
           OR NEW.annotation_taxonomy_content_sha256 IS NULL
           OR NEW.gold_production_binding_root_hash IS NULL THEN
            RAISE EXCEPTION 'Gold DatasetCertification requires frozen Gold production proof';
        END IF;

        SELECT g.workspace_id, g.output_dataset_version_id, g.annotation_snapshot_id,
               g.snapshot_root_hash, g.schema_content_sha256, g.taxonomy_content_sha256,
               g.root_hash, g.status,
               s.status, s.root_hash,
               c.schema_content_sha256, c.taxonomy_content_sha256
          INTO gold_binding_workspace, gold_binding_output_version, gold_binding_snapshot,
               gold_binding_snapshot_root, gold_binding_schema_hash, gold_binding_taxonomy_hash,
               gold_binding_root_hash, gold_binding_status,
               gold_snapshot_status, gold_snapshot_root,
               gold_campaign_schema_hash, gold_campaign_taxonomy_hash
          FROM gold_production_binding g
          JOIN annotation_snapshot s ON s.id=g.annotation_snapshot_id
          JOIN annotation_campaign c ON c.id=g.annotation_campaign_id
         WHERE g.id=NEW.gold_production_binding_id
         FOR SHARE OF g, s, c;

        IF gold_binding_workspace IS DISTINCT FROM NEW.workspace_id
           OR gold_binding_output_version IS DISTINCT FROM NEW.dataset_version_id
           OR gold_binding_status IS DISTINCT FROM 'FINALIZED'
           OR gold_binding_snapshot IS DISTINCT FROM NEW.annotation_snapshot_id
           OR gold_binding_snapshot_root IS DISTINCT FROM NEW.annotation_snapshot_root_hash
           OR gold_binding_schema_hash IS DISTINCT FROM NEW.annotation_schema_content_sha256
           OR gold_binding_taxonomy_hash IS DISTINCT FROM NEW.annotation_taxonomy_content_sha256
           OR gold_binding_root_hash IS DISTINCT FROM NEW.gold_production_binding_root_hash
           OR gold_snapshot_status IS DISTINCT FROM 'FINALIZED'
           OR gold_snapshot_root IS DISTINCT FROM NEW.annotation_snapshot_root_hash
           OR gold_campaign_schema_hash IS DISTINCT FROM NEW.annotation_schema_content_sha256
           OR gold_campaign_taxonomy_hash IS DISTINCT FROM NEW.annotation_taxonomy_content_sha256 THEN
            RAISE EXCEPTION 'Gold DatasetCertification production proof does not match the exact output';
        END IF;
    ELSE
        IF NEW.gold_production_binding_id IS NOT NULL
           OR NEW.annotation_snapshot_id IS NOT NULL
           OR NEW.annotation_snapshot_root_hash IS NOT NULL
           OR NEW.annotation_schema_content_sha256 IS NOT NULL
           OR NEW.annotation_taxonomy_content_sha256 IS NOT NULL
           OR NEW.gold_production_binding_root_hash IS NOT NULL THEN
            RAISE EXCEPTION 'non-Gold CertificationProfile cannot carry Gold production proof';
        END IF;
    END IF;

    IF NEW.compliance_result_id IS NOT NULL THEN
        SELECT workspace_id, dataset_version_id, gate_decision
          INTO compliance_workspace, compliance_dataset_version, compliance_gate
          FROM compliance_result WHERE id = NEW.compliance_result_id
          FOR SHARE;
        IF compliance_workspace IS DISTINCT FROM NEW.workspace_id OR compliance_dataset_version IS DISTINCT FROM NEW.dataset_version_id THEN
            RAISE EXCEPTION 'DatasetCertification compliance result does not match target';
        END IF;
        IF NEW.decision = 'CERTIFIED' AND profile_compliance_required AND compliance_gate <> 'PASS' THEN
            RAISE EXCEPTION 'DatasetCertification requires a passing ComplianceResult';
        END IF;
    END IF;

    IF NEW.contract_version_id IS NOT NULL THEN
        SELECT c.workspace_id, c.code, cv.status
          INTO contract_workspace, contract_code_value, contract_status
          FROM contract_version cv JOIN data_contract c ON c.id = cv.contract_id
         WHERE cv.id = NEW.contract_version_id
         FOR SHARE OF cv, c;
        IF contract_workspace IS DISTINCT FROM NEW.workspace_id THEN
            RAISE EXCEPTION 'DatasetCertification contract does not match workspace';
        END IF;
        IF NEW.decision = 'CERTIFIED' AND profile_contract_required
           AND (contract_status <> 'PUBLISHED' OR contract_code_value IS DISTINCT FROM profile_contract_code) THEN
            RAISE EXCEPTION 'DatasetCertification ContractVersion does not match frozen profile contract code';
        END IF;
    END IF;

    IF NEW.traceability_evidence_id IS NOT NULL THEN
        SELECT es.workspace_id, es.object_type, es.object_id,
               EXISTS(SELECT 1 FROM evidence_snapshot_item esi WHERE esi.snapshot_id=es.id)
          INTO traceability_workspace, traceability_object_type, traceability_object_id, traceability_has_items
          FROM evidence_snapshot es
         WHERE es.id = NEW.traceability_evidence_id
         FOR SHARE OF es;
        IF traceability_workspace IS DISTINCT FROM NEW.workspace_id
           OR traceability_object_type <> 'DATASET_VERSION'
           OR traceability_object_id IS DISTINCT FROM NEW.dataset_version_id
           OR NOT traceability_has_items THEN
            RAISE EXCEPTION 'DatasetCertification traceability evidence does not match a complete DatasetVersion EvidenceSnapshot';
        END IF;
    END IF;

    IF NEW.evidence_snapshot_id IS NOT NULL THEN
        SELECT workspace_id, object_type, object_id
          INTO evidence_workspace, evidence_object_type, evidence_object_id
          FROM evidence_snapshot WHERE id = NEW.evidence_snapshot_id
          FOR SHARE;
        IF evidence_workspace IS DISTINCT FROM NEW.workspace_id
           OR evidence_object_type <> 'DATASET_VERSION'
           OR evidence_object_id IS DISTINCT FROM NEW.dataset_version_id THEN
            RAISE EXCEPTION 'DatasetCertification evidence does not match target DatasetVersion';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

