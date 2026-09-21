-- #134 CertificationProfile. The exact content bytes and normalized,
-- queryable applicability memberships are both frozen facts.
CREATE TABLE certification_profile (
    id                         uuid PRIMARY KEY,
    workspace_id               uuid NOT NULL,
    profile_ref                varchar(512) NOT NULL,
    code                       varchar(128) NOT NULL,
    name                       varchar(255) NOT NULL,
    version                    varchar(64) NOT NULL,
    content_sha256             varchar(64) NOT NULL,
    content_snapshot           text NOT NULL,
    purpose_mode               varchar(16) NOT NULL,
    action_mode                varchar(16) NOT NULL,
    consumer_mode              varchar(16) NOT NULL,
    delivery_mode              varchar(16) NOT NULL,
    quality_gate_required      boolean NOT NULL,
    rights_required            boolean NOT NULL,
    compliance_required       boolean NOT NULL,
    contract_required         boolean NOT NULL,
    traceability_required     boolean NOT NULL,
    evidence_required         boolean NOT NULL,
    membership_state           varchar(16) NOT NULL DEFAULT 'FINALIZED',
    rights_purpose_mode        varchar(16),
    rights_action_mode         varchar(16),
    rights_consumer_mode       varchar(16),
    rights_scope_mode          varchar(16),
    created_at                 timestamptz NOT NULL DEFAULT now(),
    created_by                 uuid,
    CONSTRAINT uq_certification_profile_ref_version UNIQUE(workspace_id, profile_ref, version),
    CONSTRAINT ck_certification_profile_hash CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_certification_profile_hash_matches CHECK (
        encode(digest(convert_to(content_snapshot, 'UTF8'), 'sha256'), 'hex') = content_sha256
    ),
    CONSTRAINT ck_certification_profile_modes CHECK (
        purpose_mode IN ('ANY','EXPLICIT') AND
        action_mode IN ('ANY','EXPLICIT') AND
        consumer_mode IN ('ANY','EXPLICIT') AND
        delivery_mode IN ('ANY','EXPLICIT')
    ),
    CONSTRAINT ck_certification_profile_rights_modes CHECK (
        NOT rights_required OR (
            rights_purpose_mode IN ('ANY','EXPLICIT') AND
            rights_action_mode IN ('ANY','EXPLICIT') AND
            rights_consumer_mode IN ('ANY','EXPLICIT') AND
            rights_scope_mode IN ('ANY','EXPLICIT')
        )
    ),
    CONSTRAINT ck_certification_profile_membership_state CHECK (membership_state IN ('DRAFT','FINALIZED'))
);

CREATE TABLE certification_profile_purpose (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    purpose_code varchar(128) NOT NULL,
    PRIMARY KEY(profile_id, purpose_code)
);

CREATE TABLE certification_profile_action (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    action varchar(64) NOT NULL,
    PRIMARY KEY(profile_id, action)
);

CREATE TABLE certification_profile_consumer (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    consumer_ref varchar(255) NOT NULL,
    PRIMARY KEY(profile_id, consumer_ref)
);

CREATE TABLE certification_profile_delivery (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    delivery_channel varchar(64) NOT NULL,
    PRIMARY KEY(profile_id, delivery_channel)
);

CREATE TABLE certification_profile_quality_dimension (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    dimension varchar(64) NOT NULL,
    PRIMARY KEY(profile_id, dimension)
);

CREATE TABLE certification_profile_critical_rule (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    rule_id varchar(128) NOT NULL,
    PRIMARY KEY(profile_id, rule_id)
);

CREATE TABLE certification_profile_rights_purpose (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    purpose_code varchar(128) NOT NULL,
    PRIMARY KEY(profile_id, purpose_code)
);

CREATE TABLE certification_profile_rights_action (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    action varchar(64) NOT NULL,
    PRIMARY KEY(profile_id, action)
);

CREATE TABLE certification_profile_rights_consumer (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    consumer_ref varchar(255) NOT NULL,
    PRIMARY KEY(profile_id, consumer_ref)
);

CREATE TABLE certification_profile_rights_scope (
    profile_id uuid NOT NULL REFERENCES certification_profile(id) DEFERRABLE INITIALLY DEFERRED,
    scope_type varchar(32) NOT NULL,
    scope_ref varchar(512) NOT NULL,
    PRIMARY KEY(profile_id, scope_type, scope_ref),
    CONSTRAINT ck_certification_profile_rights_scope_type CHECK (scope_type IN ('ALL_RESOURCE','OBJECT','ROW','PREFIX','POLICY')),
    CONSTRAINT ck_certification_profile_rights_scope_ref CHECK (btrim(scope_ref) <> '')
);

CREATE OR REPLACE FUNCTION prevent_certification_profile_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.membership_state <> 'DRAFT'
       OR NEW.membership_state <> 'FINALIZED'
       OR (to_jsonb(OLD) - 'membership_state') IS DISTINCT FROM (to_jsonb(NEW) - 'membership_state') THEN
        RAISE EXCEPTION 'certification_profile is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_certification_profile_immutable
BEFORE UPDATE OR DELETE ON certification_profile
FOR EACH ROW EXECUTE FUNCTION prevent_certification_profile_mutation();

CREATE OR REPLACE FUNCTION prevent_certification_profile_membership_mutation()
RETURNS trigger AS $$
DECLARE
    old_profile_id uuid;
    new_profile_id uuid;
    profile_count integer;
    profile record;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        old_profile_id := OLD.profile_id;
        new_profile_id := NEW.profile_id;
    ELSIF TG_OP = 'DELETE' THEN
        old_profile_id := OLD.profile_id;
        new_profile_id := OLD.profile_id;
    ELSE
        old_profile_id := NEW.profile_id;
        new_profile_id := NEW.profile_id;
    END IF;

    IF old_profile_id IS DISTINCT FROM new_profile_id THEN
        SELECT count(*) INTO profile_count
          FROM certification_profile
         WHERE id IN (old_profile_id, new_profile_id);
        IF profile_count <> 2 THEN
            RAISE EXCEPTION 'certification_profile membership parent is missing';
        END IF;
        FOR profile IN
            SELECT id, membership_state
              FROM certification_profile
             WHERE id IN (old_profile_id, new_profile_id)
             ORDER BY id
             FOR UPDATE
        LOOP
            IF profile.membership_state <> 'DRAFT' THEN
                RAISE EXCEPTION 'certification_profile membership is immutable';
            END IF;
        END LOOP;
    ELSE
        SELECT p.membership_state INTO profile
          FROM certification_profile p
         WHERE p.id = old_profile_id
         FOR UPDATE;
        IF NOT FOUND OR profile.membership_state <> 'DRAFT' THEN
            RAISE EXCEPTION 'certification_profile membership is immutable';
        END IF;
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'certification_profile_purpose', 'certification_profile_action',
        'certification_profile_consumer', 'certification_profile_delivery',
        'certification_profile_quality_dimension', 'certification_profile_critical_rule',
        'certification_profile_rights_purpose', 'certification_profile_rights_action',
        'certification_profile_rights_consumer', 'certification_profile_rights_scope'
    ] LOOP
        EXECUTE format('CREATE TRIGGER trg_%s_immutable BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION prevent_certification_profile_membership_mutation()', table_name, table_name);
    END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION validate_certification_profile_membership()
RETURNS trigger AS $$
DECLARE
    purpose_count integer;
    action_count integer;
    consumer_count integer;
    delivery_count integer;
    quality_count integer;
    rule_count integer;
    rights_purpose_count integer;
    rights_action_count integer;
    rights_consumer_count integer;
    rights_scope_count integer;
BEGIN
    SELECT count(*) INTO purpose_count FROM certification_profile_purpose WHERE profile_id = NEW.id;
    SELECT count(*) INTO action_count FROM certification_profile_action WHERE profile_id = NEW.id;
    SELECT count(*) INTO consumer_count FROM certification_profile_consumer WHERE profile_id = NEW.id;
    SELECT count(*) INTO delivery_count FROM certification_profile_delivery WHERE profile_id = NEW.id;
    SELECT count(*) INTO quality_count FROM certification_profile_quality_dimension WHERE profile_id = NEW.id;
    SELECT count(*) INTO rule_count FROM certification_profile_critical_rule WHERE profile_id = NEW.id;
    SELECT count(*) INTO rights_purpose_count FROM certification_profile_rights_purpose WHERE profile_id = NEW.id;
    SELECT count(*) INTO rights_action_count FROM certification_profile_rights_action WHERE profile_id = NEW.id;
    SELECT count(*) INTO rights_consumer_count FROM certification_profile_rights_consumer WHERE profile_id = NEW.id;
    SELECT count(*) INTO rights_scope_count FROM certification_profile_rights_scope WHERE profile_id = NEW.id;

    IF (NEW.purpose_mode = 'EXPLICIT' AND purpose_count = 0)
       OR (NEW.purpose_mode = 'ANY' AND purpose_count <> 0)
       OR (NEW.action_mode = 'EXPLICIT' AND action_count = 0)
       OR (NEW.action_mode = 'ANY' AND action_count <> 0)
       OR (NEW.consumer_mode = 'EXPLICIT' AND consumer_count = 0)
       OR (NEW.consumer_mode = 'ANY' AND consumer_count <> 0)
       OR (NEW.delivery_mode = 'EXPLICIT' AND delivery_count = 0)
       OR (NEW.delivery_mode = 'ANY' AND delivery_count <> 0)
       OR (NEW.rights_required AND NEW.rights_purpose_mode = 'EXPLICIT' AND rights_purpose_count = 0)
       OR (NEW.rights_required AND NEW.rights_purpose_mode = 'ANY' AND rights_purpose_count <> 0)
       OR (NEW.rights_required AND NEW.rights_action_mode = 'EXPLICIT' AND rights_action_count = 0)
       OR (NEW.rights_required AND NEW.rights_action_mode = 'ANY' AND rights_action_count <> 0)
       OR (NEW.rights_required AND NEW.rights_consumer_mode = 'EXPLICIT' AND rights_consumer_count = 0)
       OR (NEW.rights_required AND NEW.rights_consumer_mode = 'ANY' AND rights_consumer_count <> 0)
       OR (NEW.rights_required AND NEW.rights_scope_mode = 'EXPLICIT' AND rights_scope_count = 0)
       OR (NEW.rights_required AND NEW.rights_scope_mode = 'ANY' AND rights_scope_count <> 0) THEN
        RAISE EXCEPTION 'certification_profile applicability membership is incomplete or ambiguous';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_certification_profile_membership_complete
AFTER INSERT ON certification_profile
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_certification_profile_membership();

CREATE INDEX idx_certification_profile_lookup
    ON certification_profile(workspace_id, profile_ref, version, created_at, id);
