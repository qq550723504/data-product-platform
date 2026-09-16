CREATE TABLE quality_result (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    dataset_version_id  uuid NOT NULL REFERENCES dataset_version(id),
    rule_set_ref        varchar(1024) NOT NULL,
    rule_set_version    varchar(64) NOT NULL,
    gate_decision       varchar(32) NOT NULL,
    metrics             jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    CONSTRAINT ck_quality_gate_decision CHECK(gate_decision IN ('PASS','PASS_WITH_WARNING','REVIEW','FAIL'))
);

CREATE INDEX idx_quality_result_dataset_version
    ON quality_result(dataset_version_id, created_at DESC);

CREATE TABLE quality_finding (
    id              uuid PRIMARY KEY,
    result_id       uuid NOT NULL REFERENCES quality_result(id),
    rule_id         varchar(128) NOT NULL,
    dimension       varchar(64),
    severity        varchar(32) NOT NULL,
    status          varchar(16) NOT NULL,
    observed        jsonb NOT NULL DEFAULT '{}'::jsonb,
    message         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_quality_finding_status CHECK(status IN ('PASS','FAIL','SKIPPED'))
);

CREATE INDEX idx_quality_finding_result ON quality_finding(result_id, rule_id);

CREATE TABLE compliance_result (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    dataset_version_id  uuid NOT NULL REFERENCES dataset_version(id),
    policy_ref          varchar(1024) NOT NULL,
    policy_version      varchar(64) NOT NULL,
    gate_decision       varchar(32) NOT NULL,
    summary             jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    CONSTRAINT ck_compliance_gate_decision CHECK(gate_decision IN ('PASS','REVIEW','FAIL'))
);

CREATE INDEX idx_compliance_result_dataset_version
    ON compliance_result(dataset_version_id, created_at DESC);

CREATE TABLE compliance_finding (
    id              uuid PRIMARY KEY,
    result_id       uuid NOT NULL REFERENCES compliance_result(id),
    field_name      varchar(255) NOT NULL,
    category        varchar(128),
    action          varchar(32) NOT NULL,
    status          varchar(16) NOT NULL,
    message         text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_compliance_action CHECK(action IN ('KEEP','MASK','TOKENIZE','PSEUDONYMIZE','ANONYMIZE','AGGREGATE','REMOVE','BLOCK','REVIEW')),
    CONSTRAINT ck_compliance_finding_status CHECK(status IN ('PASS','FAIL','REVIEW'))
);

CREATE INDEX idx_compliance_finding_result ON compliance_finding(result_id, field_name);

ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_quality_result
    FOREIGN KEY (quality_result_id) REFERENCES quality_result(id);

ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_compliance_result
    FOREIGN KEY (compliance_result_id) REFERENCES compliance_result(id);

CREATE OR REPLACE FUNCTION prevent_quality_result_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'quality_result is historical and immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_quality_result_immutable_update
BEFORE UPDATE OR DELETE ON quality_result
FOR EACH ROW EXECUTE FUNCTION prevent_quality_result_mutation();

CREATE OR REPLACE FUNCTION prevent_compliance_result_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'compliance_result is historical and immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_compliance_result_immutable_update
BEFORE UPDATE OR DELETE ON compliance_result
FOR EACH ROW EXECUTE FUNCTION prevent_compliance_result_mutation();
