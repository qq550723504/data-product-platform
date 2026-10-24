-- Reuses OIDC candidate identity schema; migration 51 is already owned here.
CREATE TABLE platform_actor_state (
    actor_id uuid PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT true,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE platform_subject_binding (
    id uuid PRIMARY KEY,
    issuer text NOT NULL CHECK (length(issuer) BETWEEN 1 AND 2048),
    subject varchar(255) NOT NULL CHECK (btrim(subject) <> ''),
    actor_id uuid NOT NULL REFERENCES platform_actor_state(actor_id),
    enabled boolean NOT NULL DEFAULT true,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject),
    CHECK (valid_until IS NULL OR valid_until > valid_from)
);

CREATE TABLE platform_workspace_grant (
    id uuid PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES platform_actor_state(actor_id),
    workspace_id uuid NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    capabilities text[] NOT NULL,
    valid_from timestamptz NOT NULL,
    valid_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (actor_id, workspace_id),
    CHECK (valid_until IS NULL OR valid_until > valid_from),
    CHECK (capabilities <@ ARRAY['HUMAN_DECISION','IDENTITY_BINDING_ADMIN','BUSINESS_ACCEPTANCE_READ','BUSINESS_ACCEPTANCE_DRAFT','BUSINESS_ACCEPTANCE_BASIS_CONFIRM','BUSINESS_RECEIPT_RECORD']::text[]),
    CHECK (array_position(capabilities, NULL) IS NULL)
);

CREATE FUNCTION protect_platform_identity_mapping() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'Platform identity history cannot be deleted';
    END IF;
    IF TG_TABLE_NAME = 'platform_actor_state' THEN
        IF NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION 'Platform actor identity is immutable';
        END IF;
    ELSIF TG_TABLE_NAME = 'platform_subject_binding' THEN
        IF (NEW.id,NEW.issuer,NEW.subject,NEW.actor_id,NEW.created_at) IS DISTINCT FROM
           (OLD.id,OLD.issuer,OLD.subject,OLD.actor_id,OLD.created_at) THEN
            RAISE EXCEPTION 'External subject mapping is immutable';
        END IF;
    ELSE
        IF (NEW.id,NEW.actor_id,NEW.workspace_id,NEW.created_at) IS DISTINCT FROM
           (OLD.id,OLD.actor_id,OLD.workspace_id,OLD.created_at) THEN
            RAISE EXCEPTION 'Workspace grant identity is immutable';
        END IF;
    END IF;
    IF NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'Identity state transitions require the next revision';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER protect_platform_actor BEFORE UPDATE OR DELETE ON platform_actor_state
    FOR EACH ROW EXECUTE FUNCTION protect_platform_identity_mapping();
CREATE TRIGGER protect_platform_subject BEFORE UPDATE OR DELETE ON platform_subject_binding
    FOR EACH ROW EXECUTE FUNCTION protect_platform_identity_mapping();
CREATE TRIGGER protect_platform_workspace_grant BEFORE UPDATE OR DELETE ON platform_workspace_grant
    FOR EACH ROW EXECUTE FUNCTION protect_platform_identity_mapping();

-- Same namespace as the imported IAM authority owner. Technical identity
-- mutations serialize with request transactions; no account/password service.
CREATE FUNCTION lock_platform_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(810051001::bigint);
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER aa_platform_actor_fence BEFORE INSERT OR UPDATE OR DELETE ON platform_actor_state FOR EACH ROW EXECUTE FUNCTION lock_platform_identity_mutation();
CREATE TRIGGER aa_platform_subject_fence BEFORE INSERT OR UPDATE OR DELETE ON platform_subject_binding FOR EACH ROW EXECUTE FUNCTION lock_platform_identity_mutation();
CREATE TRIGGER aa_platform_grant_fence BEFORE INSERT OR UPDATE OR DELETE ON platform_workspace_grant FOR EACH ROW EXECUTE FUNCTION lock_platform_identity_mutation();

CREATE FUNCTION lock_acceptance_binding_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock_shared(810051001::bigint);
 PERFORM pg_advisory_xact_lock(hashtextextended('acceptance-binding:'||NEW.workspace_id::text,0));
 RETURN NEW;
END; $$;
CREATE TRIGGER aa_acceptance_representation_fence BEFORE INSERT ON acceptance_party_representation FOR EACH ROW EXECUTE FUNCTION lock_acceptance_binding_mutation();
CREATE TRIGGER aa_acceptance_responsibility_fence BEFORE INSERT ON acceptance_responsibility_binding FOR EACH ROW EXECUTE FUNCTION lock_acceptance_binding_mutation();
CREATE TRIGGER aa_acceptance_applicability_fence BEFORE INSERT ON acceptance_standard_applicability FOR EACH ROW EXECUTE FUNCTION lock_acceptance_binding_mutation();
CREATE TRIGGER aa_acceptance_disposition_fence BEFORE INSERT ON acceptance_binding_disposition FOR EACH ROW EXECUTE FUNCTION lock_acceptance_binding_mutation();
