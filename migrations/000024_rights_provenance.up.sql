-- #137 Data Rights Provenance.
-- Gate-critical rights facts are normalized and append-only. JSONB remains an
-- extension field, never the authority for entitlement decisions.

ALTER TABLE authorization_resource
    ADD COLUMN scope_type varchar(32),
    ADD COLUMN scope_ref varchar(512);

ALTER TABLE authorization_resource
    ADD CONSTRAINT ck_authorization_resource_normalized_scope CHECK (
        (scope_type IS NULL AND scope_ref IS NULL)
        OR
        (scope_type IN ('ALL_RESOURCE', 'OBJECT', 'ROW', 'PREFIX', 'POLICY')
         AND scope_ref IS NOT NULL AND btrim(scope_ref) <> '')
    );

CREATE INDEX idx_authorization_resource_scope
    ON authorization_resource(data_resource_id, scope_type, scope_ref);

ALTER TABLE rights_snapshot
    ADD COLUMN status varchar(16) NOT NULL DEFAULT 'FINALIZED';

ALTER TABLE rights_snapshot
    ADD CONSTRAINT ck_rights_snapshot_status CHECK (status IN ('BUILDING', 'FINALIZED'));

CREATE TABLE rights_declaration (
    id                    uuid PRIMARY KEY,
    workspace_id          uuid NOT NULL,
    data_resource_id      uuid NOT NULL REFERENCES data_resource(id),
    claimant_ref          varchar(255) NOT NULL,
    basis_type            varchar(64) NOT NULL,
    basis_ref             varchar(512) NOT NULL,
    consumer_scope_type   varchar(16) NOT NULL DEFAULT 'ANY',
    consumer_ref          varchar(255) NOT NULL,
    effective_from        timestamptz,
    effective_to          timestamptz,
    restrictions          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            uuid,
    CONSTRAINT ck_rights_declaration_consumer CHECK (
        (consumer_scope_type = 'ANY' AND consumer_ref IS NULL)
        OR
        (consumer_scope_type = 'EXPLICIT' AND consumer_ref IS NOT NULL AND btrim(consumer_ref) <> '')
    ),
    CONSTRAINT ck_rights_declaration_validity CHECK (
        effective_to IS NULL OR effective_from IS NULL OR effective_to > effective_from
    )
);

CREATE INDEX idx_rights_declaration_resource
    ON rights_declaration(workspace_id, data_resource_id, created_at, id);

CREATE TABLE rights_declaration_party (
    id                uuid PRIMARY KEY,
    declaration_id    uuid NOT NULL REFERENCES rights_declaration(id),
    party_ref         varchar(255) NOT NULL,
    role              varchar(32) NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_rights_declaration_party UNIQUE(declaration_id, party_ref, role),
    CONSTRAINT ck_rights_declaration_party_role CHECK (
        role IN ('PROVIDER','RIGHTS_HOLDER','CUSTODIAN','CONTROLLER','PROCESSOR','AUTHORIZED_USER')
    )
);

CREATE INDEX idx_rights_declaration_party_lookup
    ON rights_declaration_party(party_ref, role, declaration_id);

CREATE TABLE rights_declaration_evidence (
    declaration_id    uuid NOT NULL REFERENCES rights_declaration(id),
    evidence_id       uuid NOT NULL REFERENCES evidence(id),
    relation_type     varchar(64) NOT NULL,
    PRIMARY KEY(declaration_id, evidence_id, relation_type)
);

CREATE INDEX idx_rights_declaration_evidence_evidence
    ON rights_declaration_evidence(evidence_id, declaration_id);

CREATE TABLE rights_declaration_permission (
    id                uuid PRIMARY KEY,
    declaration_id    uuid NOT NULL REFERENCES rights_declaration(id),
    permission_kind   varchar(16) NOT NULL,
    action            varchar(64) NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_rights_declaration_permission_kind CHECK (permission_kind IN ('USE','GRANT'))
);

CREATE INDEX idx_rights_declaration_permission_lookup
    ON rights_declaration_permission(declaration_id, permission_kind, action);

CREATE TABLE rights_declaration_purpose (
    id                uuid PRIMARY KEY,
    declaration_id    uuid NOT NULL REFERENCES rights_declaration(id),
    permission_id     uuid NOT NULL REFERENCES rights_declaration_permission(id),
    permission_kind   varchar(16) NOT NULL,
    purpose_code      varchar(128) NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_rights_declaration_purpose UNIQUE(permission_id),
    CONSTRAINT ck_rights_declaration_purpose_kind CHECK (permission_kind IN ('USE','GRANT'))
);

CREATE TABLE rights_declaration_scope (
    id                uuid PRIMARY KEY,
    declaration_id    uuid NOT NULL REFERENCES rights_declaration(id),
    permission_id     uuid NOT NULL REFERENCES rights_declaration_permission(id),
    permission_kind   varchar(16) NOT NULL,
    scope_type        varchar(32) NOT NULL,
    scope_ref         varchar(512) NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_rights_declaration_scope UNIQUE(permission_id),
    CONSTRAINT ck_rights_declaration_scope_kind CHECK (permission_kind IN ('USE','GRANT')),
    CONSTRAINT ck_rights_declaration_scope_type CHECK (scope_type IN ('ALL_RESOURCE','OBJECT','ROW','PREFIX','POLICY')),
    CONSTRAINT ck_rights_declaration_scope_ref CHECK (btrim(scope_ref) <> '')
);

CREATE TABLE rights_declaration_verification (
    id                uuid PRIMARY KEY,
    declaration_id    uuid NOT NULL UNIQUE REFERENCES rights_declaration(id),
    outcome           varchar(16) NOT NULL,
    reason            varchar(1024),
    evidence_id       uuid,
    occurred_at       timestamptz NOT NULL DEFAULT now(),
    actor_id          uuid,
    activity_id       uuid,
    CONSTRAINT ck_rights_declaration_verification_outcome CHECK (outcome IN ('VERIFIED','REJECTED'))
);

CREATE TABLE rights_declaration_disposition (
    id                         uuid PRIMARY KEY,
    declaration_id             uuid NOT NULL REFERENCES rights_declaration(id),
    disposition                varchar(16) NOT NULL,
    effective_at               timestamptz NOT NULL,
    reason                     varchar(1024) NOT NULL,
    superseded_by_declaration_id uuid REFERENCES rights_declaration(id),
    evidence_id                uuid,
    activity_id                uuid,
    occurred_at                timestamptz NOT NULL DEFAULT now(),
    actor_id                   uuid,
    CONSTRAINT ck_rights_declaration_disposition CHECK (disposition IN ('INVALIDATED','SUPERSEDED')),
    CONSTRAINT ck_rights_declaration_superseded_by CHECK (
        disposition <> 'SUPERSEDED' OR superseded_by_declaration_id IS NOT NULL
    ),
    CONSTRAINT uq_rights_declaration_disposition_activity UNIQUE(declaration_id, disposition, activity_id)
);

CREATE INDEX idx_rights_declaration_disposition_current
    ON rights_declaration_disposition(declaration_id, effective_at);

CREATE TABLE grantor_authority_delegation_chain (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL,
    source_declaration_id uuid NOT NULL REFERENCES rights_declaration(id),
    status            varchar(16) NOT NULL DEFAULT 'DRAFT',
    chain_hash        varchar(64),
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid,
    finalized_at      timestamptz,
    CONSTRAINT ck_grantor_chain_status CHECK (status IN ('DRAFT','FINALIZED'))
);

CREATE TABLE grantor_authority_delegation_edge (
    id                uuid PRIMARY KEY,
    chain_id          uuid NOT NULL REFERENCES grantor_authority_delegation_chain(id),
    ordinal           integer NOT NULL,
    delegator_ref     varchar(255) NOT NULL,
    delegate_ref      varchar(255) NOT NULL,
    data_resource_id  uuid NOT NULL REFERENCES data_resource(id),
    grantable_actions text[] NOT NULL DEFAULT ARRAY[]::text[],
    grantable_purposes text[] NOT NULL DEFAULT ARRAY[]::text[],
    scope_type        varchar(32) NOT NULL,
    scope_ref         varchar(512) NOT NULL,
    valid_from        timestamptz,
    valid_to          timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_grantor_chain_edge_ordinal UNIQUE(chain_id, ordinal),
    CONSTRAINT ck_grantor_chain_edge_order CHECK (ordinal >= 0),
    CONSTRAINT ck_grantor_chain_edge_distinct CHECK (delegator_ref <> delegate_ref),
    CONSTRAINT ck_grantor_chain_edge_scope_type CHECK (scope_type IN ('ALL_RESOURCE','OBJECT','ROW','PREFIX','POLICY')),
    CONSTRAINT ck_grantor_chain_edge_scope_ref CHECK (btrim(scope_ref) <> ''),
    CONSTRAINT ck_grantor_chain_edge_validity CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from)
);

CREATE INDEX idx_grantor_chain_edge_chain ON grantor_authority_delegation_edge(chain_id, ordinal);

CREATE TABLE grantor_authority_delegation_disposition (
    id                uuid PRIMARY KEY,
    chain_id          uuid NOT NULL REFERENCES grantor_authority_delegation_chain(id),
    edge_id           uuid REFERENCES grantor_authority_delegation_edge(id),
    disposition       varchar(16) NOT NULL,
    effective_at      timestamptz NOT NULL,
    reason            varchar(1024) NOT NULL,
    evidence_id       uuid,
    activity_id       uuid,
    occurred_at       timestamptz NOT NULL DEFAULT now(),
    actor_id          uuid,
    CONSTRAINT ck_grantor_chain_disposition CHECK (disposition IN ('REVOKED','INVALIDATED','SUPERSEDED'))
);

CREATE TABLE authorization_provenance_binding (
    id                    uuid PRIMARY KEY,
    workspace_id          uuid NOT NULL,
    authorization_id      uuid NOT NULL REFERENCES data_authorization(id),
    data_resource_id      uuid NOT NULL REFERENCES data_resource(id),
    rights_declaration_id uuid NOT NULL REFERENCES rights_declaration(id),
    grantor_ref            varchar(255) NOT NULL,
    grantor_authority_mode varchar(32) NOT NULL,
    delegation_chain_id    uuid REFERENCES grantor_authority_delegation_chain(id),
    delegation_chain_hash  varchar(64),
    created_at             timestamptz NOT NULL DEFAULT now(),
    created_by             uuid,
    CONSTRAINT ck_binding_authority_mode CHECK (grantor_authority_mode IN ('DIRECT_DECLARATION_PARTY','DELEGATED')),
    CONSTRAINT ck_binding_delegation_identity CHECK (
        (grantor_authority_mode = 'DIRECT_DECLARATION_PARTY' AND delegation_chain_id IS NULL AND delegation_chain_hash IS NULL)
        OR
        (grantor_authority_mode = 'DELEGATED' AND delegation_chain_id IS NOT NULL AND delegation_chain_hash IS NOT NULL)
    )
);

CREATE INDEX idx_binding_authorization ON authorization_provenance_binding(authorization_id, created_at, id);
CREATE INDEX idx_binding_declaration ON authorization_provenance_binding(rights_declaration_id, created_at, id);

CREATE TABLE authorization_provenance_binding_disposition (
    id                uuid PRIMARY KEY,
    binding_id        uuid NOT NULL REFERENCES authorization_provenance_binding(id),
    disposition       varchar(16) NOT NULL,
    effective_at      timestamptz NOT NULL,
    reason            varchar(1024) NOT NULL,
    superseded_by_binding_id uuid REFERENCES authorization_provenance_binding(id),
    evidence_id       uuid,
    activity_id       uuid,
    occurred_at       timestamptz NOT NULL DEFAULT now(),
    actor_id          uuid,
    CONSTRAINT ck_binding_disposition CHECK (disposition IN ('INVALIDATED','SUPERSEDED')),
    CONSTRAINT ck_binding_superseded_by CHECK (
        disposition <> 'SUPERSEDED' OR superseded_by_binding_id IS NOT NULL
    ),
    CONSTRAINT uq_binding_disposition_activity UNIQUE(binding_id, disposition, activity_id)
);

CREATE INDEX idx_binding_disposition_current
    ON authorization_provenance_binding_disposition(binding_id, effective_at);

CREATE TABLE rights_snapshot_declaration (
    rights_snapshot_id    uuid NOT NULL REFERENCES rights_snapshot(id),
    declaration_id        uuid NOT NULL REFERENCES rights_declaration(id),
    PRIMARY KEY(rights_snapshot_id, declaration_id)
);

CREATE TABLE rights_snapshot_provenance_binding (
    rights_snapshot_id    uuid NOT NULL REFERENCES rights_snapshot(id),
    binding_id            uuid NOT NULL REFERENCES authorization_provenance_binding(id),
    PRIMARY KEY(rights_snapshot_id, binding_id)
);

CREATE TABLE effective_rights_snapshot (
    id                    uuid PRIMARY KEY,
    workspace_id          uuid NOT NULL,
    target_dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
    calculation_as_of     timestamptz NOT NULL,
    consumer_ref          varchar(255),
    purpose               varchar(128) NOT NULL,
    calculation_rule_version varchar(64) NOT NULL,
    calculation_rule_hash varchar(64) NOT NULL,
    required_input_hash   varchar(64) NOT NULL,
    status                varchar(16) NOT NULL DEFAULT 'DRAFT',
    root_hash             varchar(64),
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            uuid,
    finalized_at          timestamptz,
    CONSTRAINT ck_effective_rights_status CHECK (status IN ('DRAFT','FINALIZED'))
);

CREATE INDEX idx_effective_rights_target ON effective_rights_snapshot(target_dataset_version_id, created_at, id);

CREATE TABLE effective_rights_input (
    id                    uuid PRIMARY KEY,
    snapshot_id           uuid NOT NULL REFERENCES effective_rights_snapshot(id),
    input_dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
    data_resource_id      uuid NOT NULL REFERENCES data_resource(id),
    rights_snapshot_id    uuid REFERENCES rights_snapshot(id),
    declaration_id        uuid REFERENCES rights_declaration(id),
    binding_id            uuid REFERENCES authorization_provenance_binding(id),
    input_hash            varchar(64) NOT NULL,
    CONSTRAINT uq_effective_rights_input UNIQUE(snapshot_id, input_dataset_version_id),
    CONSTRAINT ck_effective_rights_input_provenance CHECK (
        (rights_snapshot_id IS NULL AND declaration_id IS NULL AND binding_id IS NULL)
        OR declaration_id IS NOT NULL
    )
);

CREATE TABLE effective_rights_action (
    id                    uuid PRIMARY KEY,
    snapshot_id           uuid NOT NULL REFERENCES effective_rights_snapshot(id),
    action                varchar(64) NOT NULL,
    decision              varchar(16) NOT NULL,
    reason                varchar(1024) NOT NULL,
    blocking_input_id     uuid REFERENCES effective_rights_input(id),
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_effective_rights_action UNIQUE(snapshot_id, action),
    CONSTRAINT ck_effective_rights_action_decision CHECK (decision IN ('ALLOWED','NOT_ALLOWED'))
);

CREATE TABLE effective_rights_action_provenance (
    snapshot_id       uuid NOT NULL REFERENCES effective_rights_snapshot(id),
    action_id         uuid NOT NULL REFERENCES effective_rights_action(id),
    input_id          uuid NOT NULL REFERENCES effective_rights_input(id),
    declaration_id    uuid NOT NULL REFERENCES rights_declaration(id),
    PRIMARY KEY(action_id, input_id)
);

-- Subject-typed CostAllocation keeps rights activity cost auditable without
-- putting business IDs into cost_event.metadata.
ALTER TABLE cost_allocation
    ADD COLUMN rights_declaration_id uuid REFERENCES rights_declaration(id),
    ADD COLUMN rights_declaration_verification_id uuid REFERENCES rights_declaration_verification(id),
    ADD COLUMN rights_declaration_disposition_id uuid REFERENCES rights_declaration_disposition(id),
    ADD COLUMN authorization_provenance_binding_id uuid REFERENCES authorization_provenance_binding(id),
    ADD COLUMN authorization_provenance_binding_disposition_id uuid REFERENCES authorization_provenance_binding_disposition(id),
    ADD COLUMN effective_rights_snapshot_id uuid REFERENCES effective_rights_snapshot(id);

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
    (CASE WHEN effective_rights_snapshot_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE INDEX idx_cost_allocation_rights_declaration ON cost_allocation(rights_declaration_id) WHERE rights_declaration_id IS NOT NULL;
CREATE INDEX idx_cost_allocation_rights_verification ON cost_allocation(rights_declaration_verification_id) WHERE rights_declaration_verification_id IS NOT NULL;
CREATE INDEX idx_cost_allocation_rights_binding ON cost_allocation(authorization_provenance_binding_id) WHERE authorization_provenance_binding_id IS NOT NULL;

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
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_verification v JOIN rights_declaration d ON d.id=v.declaration_id
         WHERE v.id=NEW.rights_declaration_verification_id;
    ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_disposition x JOIN rights_declaration d ON d.id=x.declaration_id
         WHERE x.id=NEW.rights_declaration_disposition_id;
    ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM authorization_provenance_binding WHERE id=NEW.authorization_provenance_binding_id;
    ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
        SELECT b.workspace_id INTO subject_workspace
          FROM authorization_provenance_binding_disposition x JOIN authorization_provenance_binding b ON b.id=x.binding_id
         WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
    ELSE
        SELECT workspace_id INTO subject_workspace FROM effective_rights_snapshot WHERE id=NEW.effective_rights_snapshot_id;
    END IF;
    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cost_allocation_rights_workspace
BEFORE INSERT ON cost_allocation
FOR EACH ROW EXECUTE FUNCTION guard_cost_allocation_workspace();

CREATE OR REPLACE FUNCTION prevent_rights_snapshot_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.status = 'BUILDING' AND NEW.status = 'FINALIZED'
       AND OLD.id IS NOT DISTINCT FROM NEW.id
       AND OLD.workspace_id IS NOT DISTINCT FROM NEW.workspace_id
       AND OLD.product_release_id IS NOT DISTINCT FROM NEW.product_release_id
       AND OLD.purpose IS NOT DISTINCT FROM NEW.purpose
       AND OLD.consumer_ref IS NOT DISTINCT FROM NEW.consumer_ref
       AND OLD.as_of IS NOT DISTINCT FROM NEW.as_of
       AND OLD.manifest IS NOT DISTINCT FROM NEW.manifest
       AND OLD.root_hash IS NOT DISTINCT FROM NEW.root_hash
       AND OLD.created_at IS NOT DISTINCT FROM NEW.created_at
       AND OLD.created_by IS NOT DISTINCT FROM NEW.created_by THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'rights_snapshot is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION guard_rights_snapshot_membership_mutation()
RETURNS trigger AS $$
DECLARE
    parent record;
    old_snapshot_id uuid;
    new_snapshot_id uuid;
BEGIN
    old_snapshot_id := CASE WHEN TG_OP = 'INSERT' THEN NULL ELSE OLD.rights_snapshot_id END;
    new_snapshot_id := CASE WHEN TG_OP = 'DELETE' THEN NULL ELSE NEW.rights_snapshot_id END;
    FOR parent IN
        SELECT id,status FROM rights_snapshot
        WHERE id IN (old_snapshot_id,new_snapshot_id)
        ORDER BY id
        FOR UPDATE
    LOOP
        IF parent.status IS DISTINCT FROM 'BUILDING' THEN
            RAISE EXCEPTION 'finalized rights_snapshot membership is immutable';
        END IF;
    END LOOP;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rights_snapshot_authorization_immutable
BEFORE INSERT OR UPDATE OR DELETE ON rights_snapshot_authorization
FOR EACH ROW EXECUTE FUNCTION guard_rights_snapshot_membership_mutation();
CREATE TRIGGER trg_rights_snapshot_declaration_immutable
BEFORE INSERT OR UPDATE OR DELETE ON rights_snapshot_declaration
FOR EACH ROW EXECUTE FUNCTION guard_rights_snapshot_membership_mutation();
CREATE TRIGGER trg_rights_snapshot_binding_immutable
BEFORE INSERT OR UPDATE OR DELETE ON rights_snapshot_provenance_binding
FOR EACH ROW EXECUTE FUNCTION guard_rights_snapshot_membership_mutation();

CREATE OR REPLACE FUNCTION guard_rights_snapshot_header_status()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' OR OLD.status <> 'BUILDING' THEN
        RAISE EXCEPTION 'rights_snapshot is immutable';
    END IF;
    IF NEW.status = 'BUILDING'
       AND OLD.id IS NOT DISTINCT FROM NEW.id
       AND OLD.workspace_id IS NOT DISTINCT FROM NEW.workspace_id
       AND OLD.product_release_id IS NOT DISTINCT FROM NEW.product_release_id
       AND OLD.purpose IS NOT DISTINCT FROM NEW.purpose
       AND OLD.consumer_ref IS NOT DISTINCT FROM NEW.consumer_ref
       AND OLD.as_of IS NOT DISTINCT FROM NEW.as_of
       AND OLD.manifest IS NOT DISTINCT FROM NEW.manifest
       AND OLD.created_at IS NOT DISTINCT FROM NEW.created_at
       AND OLD.created_by IS NOT DISTINCT FROM NEW.created_by THEN
        RETURN NEW;
    END IF;
    IF NEW.status <> 'FINALIZED'
       OR OLD.id IS DISTINCT FROM NEW.id
       OR OLD.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR OLD.product_release_id IS DISTINCT FROM NEW.product_release_id
       OR OLD.purpose IS DISTINCT FROM NEW.purpose
       OR OLD.consumer_ref IS DISTINCT FROM NEW.consumer_ref
       OR OLD.as_of IS DISTINCT FROM NEW.as_of
       OR OLD.manifest IS DISTINCT FROM NEW.manifest
       OR OLD.created_at IS DISTINCT FROM NEW.created_at
       OR OLD.created_by IS DISTINCT FROM NEW.created_by THEN
        RAISE EXCEPTION 'rights_snapshot status transition is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_rights_snapshot_immutable_update ON rights_snapshot;
DROP TRIGGER IF EXISTS trg_rights_snapshot_immutable_delete ON rights_snapshot;
CREATE TRIGGER trg_rights_snapshot_immutable_update
BEFORE UPDATE ON rights_snapshot
FOR EACH ROW EXECUTE FUNCTION guard_rights_snapshot_header_status();
CREATE TRIGGER trg_rights_snapshot_immutable_delete
BEFORE DELETE ON rights_snapshot
FOR EACH ROW EXECUTE FUNCTION guard_rights_snapshot_header_status();

CREATE OR REPLACE FUNCTION guard_effective_rights_header_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'effective_rights_snapshot is immutable'; END IF;
    IF OLD.status='DRAFT' AND NEW.status='FINALIZED'
       AND OLD.id IS NOT DISTINCT FROM NEW.id
       AND OLD.workspace_id IS NOT DISTINCT FROM NEW.workspace_id
       AND OLD.target_dataset_version_id IS NOT DISTINCT FROM NEW.target_dataset_version_id
       AND OLD.calculation_as_of IS NOT DISTINCT FROM NEW.calculation_as_of
       AND OLD.consumer_ref IS NOT DISTINCT FROM NEW.consumer_ref
       AND OLD.purpose IS NOT DISTINCT FROM NEW.purpose
       AND OLD.calculation_rule_version IS NOT DISTINCT FROM NEW.calculation_rule_version
       AND OLD.calculation_rule_hash IS NOT DISTINCT FROM NEW.calculation_rule_hash
       AND OLD.required_input_hash IS NOT DISTINCT FROM NEW.required_input_hash
       AND OLD.root_hash IS NOT DISTINCT FROM NEW.root_hash
       AND OLD.created_at IS NOT DISTINCT FROM NEW.created_at
       AND OLD.created_by IS NOT DISTINCT FROM NEW.created_by THEN RETURN NEW; END IF;
    RAISE EXCEPTION 'effective_rights_snapshot is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION guard_effective_rights_membership_mutation()
RETURNS trigger AS $$
DECLARE
    parent record;
    old_snapshot_id uuid;
    new_snapshot_id uuid;
BEGIN
    old_snapshot_id := CASE WHEN TG_OP='INSERT' THEN NULL ELSE OLD.snapshot_id END;
    new_snapshot_id := CASE WHEN TG_OP='DELETE' THEN NULL ELSE NEW.snapshot_id END;
    FOR parent IN
        SELECT id,status FROM effective_rights_snapshot
        WHERE id IN (old_snapshot_id,new_snapshot_id)
        ORDER BY id
        FOR UPDATE
    LOOP
        IF parent.status IS DISTINCT FROM 'DRAFT' THEN
            RAISE EXCEPTION 'finalized effective rights membership is immutable';
        END IF;
    END LOOP;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_effective_rights_header_immutable
BEFORE UPDATE OR DELETE ON effective_rights_snapshot
FOR EACH ROW EXECUTE FUNCTION guard_effective_rights_header_mutation();
CREATE TRIGGER trg_effective_rights_input_immutable
BEFORE INSERT OR UPDATE OR DELETE ON effective_rights_input
FOR EACH ROW EXECUTE FUNCTION guard_effective_rights_membership_mutation();
CREATE TRIGGER trg_effective_rights_action_immutable
BEFORE INSERT OR UPDATE OR DELETE ON effective_rights_action
FOR EACH ROW EXECUTE FUNCTION guard_effective_rights_membership_mutation();
CREATE TRIGGER trg_effective_rights_action_provenance_immutable
BEFORE INSERT OR UPDATE OR DELETE ON effective_rights_action_provenance
FOR EACH ROW EXECUTE FUNCTION guard_effective_rights_membership_mutation();

CREATE OR REPLACE FUNCTION guard_delegation_chain_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'delegation chain is immutable'; END IF;
    IF OLD.status='DRAFT' AND NEW.status='FINALIZED'
       AND OLD.id IS NOT DISTINCT FROM NEW.id
       AND OLD.workspace_id IS NOT DISTINCT FROM NEW.workspace_id
       AND OLD.source_declaration_id IS NOT DISTINCT FROM NEW.source_declaration_id
       AND (OLD.chain_hash IS NOT DISTINCT FROM NEW.chain_hash OR (OLD.chain_hash IS NULL AND NEW.chain_hash IS NOT NULL))
       AND OLD.created_at IS NOT DISTINCT FROM NEW.created_at
       AND OLD.created_by IS NOT DISTINCT FROM NEW.created_by THEN RETURN NEW; END IF;
    RAISE EXCEPTION 'delegation chain is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION guard_delegation_edge_mutation()
RETURNS trigger AS $$
DECLARE
    parent record;
    old_chain_id uuid;
    new_chain_id uuid;
BEGIN
    old_chain_id := CASE WHEN TG_OP='INSERT' THEN NULL ELSE OLD.chain_id END;
    new_chain_id := CASE WHEN TG_OP='DELETE' THEN NULL ELSE NEW.chain_id END;
    FOR parent IN
        SELECT id,status FROM grantor_authority_delegation_chain
        WHERE id IN (old_chain_id,new_chain_id)
        ORDER BY id
        FOR UPDATE
    LOOP
        IF parent.status IS DISTINCT FROM 'DRAFT' THEN
            RAISE EXCEPTION 'finalized delegation chain membership is immutable';
        END IF;
    END LOOP;
    RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_delegation_chain_immutable
BEFORE UPDATE OR DELETE ON grantor_authority_delegation_chain
FOR EACH ROW EXECUTE FUNCTION guard_delegation_chain_mutation();
CREATE TRIGGER trg_delegation_edge_immutable
BEFORE INSERT OR UPDATE OR DELETE ON grantor_authority_delegation_edge
FOR EACH ROW EXECUTE FUNCTION guard_delegation_edge_mutation();

CREATE OR REPLACE FUNCTION prevent_rights_history_mutation()
RETURNS trigger AS $$
BEGIN RAISE EXCEPTION '% is append-only rights history', TG_TABLE_NAME; END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION prevent_rights_declaration_mutation()
RETURNS trigger AS $$
BEGIN RAISE EXCEPTION '% is an immutable rights declaration fact', TG_TABLE_NAME; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rights_declaration_immutable
BEFORE UPDATE OR DELETE ON rights_declaration
FOR EACH ROW EXECUTE FUNCTION prevent_rights_declaration_mutation();
CREATE TRIGGER trg_rights_declaration_party_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_party
FOR EACH ROW EXECUTE FUNCTION prevent_rights_declaration_mutation();
CREATE TRIGGER trg_rights_declaration_evidence_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_evidence
FOR EACH ROW EXECUTE FUNCTION prevent_rights_declaration_mutation();
CREATE TRIGGER trg_rights_declaration_permission_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_permission
FOR EACH ROW EXECUTE FUNCTION prevent_rights_declaration_mutation();
CREATE TRIGGER trg_rights_declaration_purpose_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_purpose
FOR EACH ROW EXECUTE FUNCTION prevent_rights_declaration_mutation();
CREATE TRIGGER trg_rights_declaration_scope_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_scope
FOR EACH ROW EXECUTE FUNCTION prevent_rights_declaration_mutation();

CREATE TRIGGER trg_rights_declaration_verification_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_verification
FOR EACH ROW EXECUTE FUNCTION prevent_rights_history_mutation();
CREATE TRIGGER trg_rights_declaration_disposition_immutable
BEFORE UPDATE OR DELETE ON rights_declaration_disposition
FOR EACH ROW EXECUTE FUNCTION prevent_rights_history_mutation();
CREATE TRIGGER trg_binding_immutable
BEFORE UPDATE OR DELETE ON authorization_provenance_binding
FOR EACH ROW EXECUTE FUNCTION prevent_rights_history_mutation();
CREATE TRIGGER trg_binding_disposition_immutable
BEFORE UPDATE OR DELETE ON authorization_provenance_binding_disposition
FOR EACH ROW EXECUTE FUNCTION prevent_rights_history_mutation();
CREATE TRIGGER trg_delegation_disposition_immutable
BEFORE UPDATE OR DELETE ON grantor_authority_delegation_disposition
FOR EACH ROW EXECUTE FUNCTION prevent_rights_history_mutation();
