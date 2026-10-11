-- #84 records Core intent/associations, never provider execution success.
CREATE TABLE external_identity_binding (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    provider varchar(64) NOT NULL CHECK (provider ~ '^[a-z][a-z0-9._-]{0,63}$'),
    subject_type varchar(32) NOT NULL CHECK (subject_type='PRINCIPAL'),
    subject_id uuid NOT NULL,
    principal_ref varchar(255) NOT NULL CHECK (length(btrim(principal_ref))>0),
    external_subject_id varchar(255) NOT NULL CHECK (length(btrim(external_subject_id))>0),
    basis_evidence_id uuid NOT NULL REFERENCES evidence(id),
    evidence_id uuid NOT NULL REFERENCES evidence(id),
    created_by uuid NOT NULL CHECK (created_by=subject_id),
    created_at timestamptz NOT NULL,
    UNIQUE (workspace_id,provider,external_subject_id)
);
CREATE TABLE external_identity_binding_disposition (
    id uuid PRIMARY KEY,
    binding_id uuid NOT NULL UNIQUE REFERENCES external_identity_binding(id),
    workspace_id uuid NOT NULL,
    disposition varchar(32) NOT NULL CHECK (disposition='INVALIDATED'),
    reason varchar(512) NOT NULL CHECK (length(btrim(reason))>0),
    evidence_id uuid NOT NULL REFERENCES evidence(id),
    principal_ref varchar(255) NOT NULL CHECK (length(btrim(principal_ref))>0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE TABLE external_identifier (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    provider varchar(64) NOT NULL CHECK (provider ~ '^[a-z][a-z0-9._-]{0,63}$'),
    object_type varchar(32) NOT NULL CHECK (object_type IN ('SUBJECT','DATA_RESOURCE','DATA_PRODUCT','PRODUCT_RELEASE')),
    subject_binding_id uuid REFERENCES external_identity_binding(id),
    subject_id uuid,
    resource_id uuid REFERENCES data_resource(id),
    product_id uuid REFERENCES data_product(id),
    release_id uuid REFERENCES product_release(id),
    object_id uuid GENERATED ALWAYS AS (COALESCE(subject_id,resource_id,product_id,release_id)) STORED,
    external_id varchar(255) NOT NULL CHECK (length(btrim(external_id))>0),
    basis_evidence_id uuid NOT NULL REFERENCES evidence(id),
    evidence_id uuid NOT NULL REFERENCES evidence(id),
    principal_ref varchar(255) NOT NULL CHECK (length(btrim(principal_ref))>0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK (
      (object_type='SUBJECT' AND subject_binding_id IS NOT NULL AND subject_id IS NOT NULL AND resource_id IS NULL AND product_id IS NULL AND release_id IS NULL) OR
      (object_type='DATA_RESOURCE' AND resource_id IS NOT NULL AND subject_binding_id IS NULL AND subject_id IS NULL AND product_id IS NULL AND release_id IS NULL) OR
      (object_type='DATA_PRODUCT' AND product_id IS NOT NULL AND subject_binding_id IS NULL AND subject_id IS NULL AND resource_id IS NULL AND release_id IS NULL) OR
      (object_type='PRODUCT_RELEASE' AND release_id IS NOT NULL AND subject_binding_id IS NULL AND subject_id IS NULL AND resource_id IS NULL AND product_id IS NULL)
    ),
    UNIQUE (workspace_id,provider,object_type,external_id)
);
CREATE TABLE external_identifier_disposition (
    id uuid PRIMARY KEY,
    identifier_id uuid NOT NULL UNIQUE REFERENCES external_identifier(id),
    workspace_id uuid NOT NULL,
    disposition varchar(32) NOT NULL CHECK (disposition='INVALIDATED'),
    reason varchar(512) NOT NULL CHECK (length(btrim(reason))>0),
    evidence_id uuid NOT NULL REFERENCES evidence(id),
    principal_ref varchar(255) NOT NULL CHECK (length(btrim(principal_ref))>0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE TABLE external_publication (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    provider varchar(64) NOT NULL CHECK (provider ~ '^[a-z][a-z0-9._-]{0,63}$'),
    identity_binding_id uuid NOT NULL REFERENCES external_identity_binding(id),
    product_release_id uuid NOT NULL REFERENCES product_release(id),
    status varchar(32) NOT NULL CHECK (status IN ('DRAFT','SUBMITTING')),
    revision bigint NOT NULL CHECK (revision IN (0,1)),
    principal_ref varchar(255) NOT NULL CHECK (length(btrim(principal_ref))>0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (workspace_id,provider,product_release_id)
);
CREATE TABLE external_publication_transition (
    id uuid PRIMARY KEY,
    publication_id uuid NOT NULL REFERENCES external_publication(id),
    workspace_id uuid NOT NULL,
    revision bigint NOT NULL,
    from_status varchar(32) NOT NULL,
    to_status varchar(32) NOT NULL,
    evidence_id uuid NOT NULL REFERENCES evidence(id),
    event_id uuid NOT NULL UNIQUE REFERENCES outbox_event(id),
    principal_ref varchar(255) NOT NULL CHECK (length(btrim(principal_ref))>0),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK ((revision=0 AND from_status='' AND to_status='DRAFT') OR
           (revision=1 AND from_status='DRAFT' AND to_status='SUBMITTING')),
    UNIQUE (publication_id,revision)
);
CREATE INDEX idx_external_publication_workspace ON external_publication(workspace_id,created_at,id);
CREATE INDEX idx_external_binding_workspace ON external_identity_binding(workspace_id,created_at,id);
CREATE INDEX idx_external_identifier_workspace ON external_identifier(workspace_id,created_at,id);

CREATE FUNCTION guard_external_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE='23514';
END $$;
DO $$ DECLARE t text; BEGIN
  FOREACH t IN ARRAY ARRAY['external_identity_binding','external_identity_binding_disposition','external_identifier','external_identifier_disposition','external_publication_transition'] LOOP
    EXECUTE format('CREATE TRIGGER immutable_external_fact BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION guard_external_immutable()',t);
  END LOOP;
END $$;

CREATE FUNCTION guard_external_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b external_identity_binding; ws uuid; actor uuid;
BEGIN
  IF TG_TABLE_NAME='external_identity_binding' THEN
    PERFORM pg_advisory_xact_lock(hashtextextended('external-binding/'||NEW.workspace_id||'/'||NEW.provider||'/'||NEW.subject_id,0));
    PERFORM 1 FROM external_identity_binding WHERE workspace_id=NEW.workspace_id AND provider=NEW.provider AND subject_id=NEW.subject_id FOR UPDATE;
    IF EXISTS(SELECT 1 FROM external_identity_binding existing WHERE existing.workspace_id=NEW.workspace_id AND existing.provider=NEW.provider AND existing.subject_id=NEW.subject_id
      AND NOT EXISTS(SELECT 1 FROM external_identity_binding_disposition x WHERE x.binding_id=existing.id)) THEN
      RAISE EXCEPTION 'current external identity binding already exists' USING ERRCODE='23505';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.basis_evidence_id AND workspace_id=NEW.workspace_id) THEN
      RAISE EXCEPTION 'external identity basis crosses workspace' USING ERRCODE='23514';
    END IF;
  ELSIF TG_TABLE_NAME='external_identity_binding_disposition' THEN
    SELECT * INTO b FROM external_identity_binding WHERE id=NEW.binding_id FOR UPDATE;
    IF b.id IS NULL OR b.workspace_id<>NEW.workspace_id THEN
      RAISE EXCEPTION 'external identity disposition crosses workspace' USING ERRCODE='23514';
    END IF;
  ELSIF TG_TABLE_NAME='external_identifier_disposition' THEN
    SELECT workspace_id INTO ws FROM external_identifier WHERE id=NEW.identifier_id FOR UPDATE;
    IF ws IS NULL OR ws<>NEW.workspace_id THEN
      RAISE EXCEPTION 'external identifier disposition crosses workspace' USING ERRCODE='23514';
    END IF;
  ELSIF TG_TABLE_NAME='external_identifier' THEN
    PERFORM pg_advisory_xact_lock(hashtextextended('external-identifier/'||NEW.workspace_id||'/'||NEW.provider||'/'||NEW.object_type||'/'||COALESCE(NEW.subject_id,NEW.resource_id,NEW.product_id,NEW.release_id),0));
    PERFORM 1 FROM external_identifier WHERE workspace_id=NEW.workspace_id AND provider=NEW.provider AND object_type=NEW.object_type AND object_id=COALESCE(NEW.subject_id,NEW.resource_id,NEW.product_id,NEW.release_id) FOR UPDATE;
    IF EXISTS(SELECT 1 FROM external_identifier i WHERE i.workspace_id=NEW.workspace_id AND i.provider=NEW.provider AND i.object_type=NEW.object_type AND i.object_id=COALESCE(NEW.subject_id,NEW.resource_id,NEW.product_id,NEW.release_id)
      AND NOT EXISTS(SELECT 1 FROM external_identifier_disposition x WHERE x.identifier_id=i.id)) THEN
      RAISE EXCEPTION 'current external identifier already exists' USING ERRCODE='23505';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.basis_evidence_id AND workspace_id=NEW.workspace_id) THEN
      RAISE EXCEPTION 'external identifier basis crosses workspace' USING ERRCODE='23514';
    END IF;
    CASE NEW.object_type
      WHEN 'SUBJECT' THEN
        SELECT * INTO b FROM external_identity_binding WHERE id=NEW.subject_binding_id FOR UPDATE;
        IF b.id IS NULL OR b.workspace_id<>NEW.workspace_id OR b.provider<>NEW.provider OR b.subject_id<>NEW.subject_id OR
          EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=b.id) THEN
          RAISE EXCEPTION 'invalid external subject binding' USING ERRCODE='23514';
        END IF;
        ws:=b.workspace_id;
      WHEN 'DATA_RESOURCE' THEN
        SELECT workspace_id INTO ws FROM data_resource WHERE id=NEW.resource_id AND deleted_at IS NULL FOR SHARE;
      WHEN 'DATA_PRODUCT' THEN
        SELECT workspace_id INTO ws FROM data_product WHERE id=NEW.product_id AND deleted_at IS NULL FOR SHARE;
      WHEN 'PRODUCT_RELEASE' THEN
        SELECT p.workspace_id INTO ws FROM product_release r JOIN data_product p ON p.id=r.product_id
          WHERE r.id=NEW.release_id FOR SHARE OF r;
      ELSE RAISE EXCEPTION 'unsupported external object' USING ERRCODE='23514';
    END CASE;
    IF ws IS NULL OR ws<>NEW.workspace_id THEN
      RAISE EXCEPTION 'external identifier crosses workspace' USING ERRCODE='23514';
    END IF;
  ELSIF TG_TABLE_NAME='external_publication' THEN
    SELECT * INTO b FROM external_identity_binding WHERE id=NEW.identity_binding_id FOR UPDATE;
    IF b.id IS NULL OR b.workspace_id<>NEW.workspace_id OR b.provider<>NEW.provider OR
       EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=b.id) THEN
      RAISE EXCEPTION 'external publication requires current matching identity' USING ERRCODE='23514';
    END IF;
    SELECT p.workspace_id INTO ws FROM product_release r JOIN data_product p ON p.id=r.product_id
      WHERE r.id=NEW.product_release_id AND r.status='PUBLISHED' FOR SHARE OF r;
    IF ws IS NULL OR ws<>NEW.workspace_id THEN
      RAISE EXCEPTION 'external publication requires PUBLISHED release in workspace' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  END IF;
  IF NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.evidence_id AND workspace_id=NEW.workspace_id) THEN
    RAISE EXCEPTION 'external command evidence crosses workspace' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
DO $$ DECLARE t text; BEGIN
  FOREACH t IN ARRAY ARRAY['external_identity_binding','external_identity_binding_disposition','external_identifier','external_identifier_disposition','external_publication'] LOOP
    EXECUTE format('CREATE TRIGGER external_reference BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard_external_reference()',t);
  END LOOP;
END $$;

CREATE FUNCTION guard_external_publication_projection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION 'external publication cannot be deleted' USING ERRCODE='23514';
  END IF;
  IF TG_OP='INSERT' THEN
    IF NEW.status<>'DRAFT' OR NEW.revision<>0 THEN
      RAISE EXCEPTION 'external publication starts DRAFT' USING ERRCODE='23514';
    END IF;
  ELSE
    IF (to_jsonb(NEW)-'status'-'revision') IS DISTINCT FROM (to_jsonb(OLD)-'status'-'revision') OR
       OLD.status<>'DRAFT' OR NEW.status<>'SUBMITTING' OR OLD.revision<>0 OR NEW.revision<>1 THEN
      RAISE EXCEPTION 'external publication permits only explicit DRAFT to SUBMITTING' USING ERRCODE='23514';
    END IF;
    -- Parent lock is already owned by UPDATE. Application locks identity first;
    -- direct DB projection writes must not wait on identity in reverse order.
    PERFORM 1 FROM external_identity_binding WHERE id=NEW.identity_binding_id FOR UPDATE NOWAIT;
    IF EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=NEW.identity_binding_id) THEN
      RAISE EXCEPTION 'external publication identity invalidated' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER external_projection BEFORE INSERT OR UPDATE OR DELETE ON external_publication
FOR EACH ROW EXECUTE FUNCTION guard_external_publication_projection();

CREATE FUNCTION guard_external_transition() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p external_publication; expected_event text;
BEGIN
  SELECT * INTO p FROM external_publication WHERE id=NEW.publication_id FOR UPDATE;
  IF p.id IS NULL OR p.workspace_id<>NEW.workspace_id OR p.revision<>NEW.revision OR p.status<>NEW.to_status OR
     NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.evidence_id AND workspace_id=p.workspace_id) THEN
    RAISE EXCEPTION 'external transition projection/evidence mismatch' USING ERRCODE='23514';
  END IF;
  expected_event:=CASE NEW.to_status WHEN 'DRAFT' THEN 'ExternalPublicationCreated' ELSE 'ExternalPublicationRequested' END;
  IF NOT EXISTS(SELECT 1 FROM outbox_event WHERE id=NEW.event_id AND aggregate_id=p.id AND
    aggregate_type='ExternalPublication' AND event_type=expected_event AND
    payload->>'workspaceId'=p.workspace_id::text AND payload->>'revision'=NEW.revision::text AND
    payload->>'factId'=NEW.id::text AND payload->>'actorId'=NEW.created_by::text AND
    payload->>'principalRef'=NEW.principal_ref) THEN
    RAISE EXCEPTION 'external transition event mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER external_transition BEFORE INSERT ON external_publication_transition
FOR EACH ROW EXECUTE FUNCTION guard_external_transition();

CREATE FUNCTION check_external_projection_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM external_publication_transition WHERE publication_id=NEW.id AND revision=NEW.revision AND to_status=NEW.status) THEN
    RAISE EXCEPTION 'external publication requires immutable transition' USING ERRCODE='23514';
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER external_projection_history AFTER INSERT OR UPDATE ON external_publication
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_external_projection_history();
