-- #294: controlled immutable source facts; no external review engine.
ALTER TABLE annotation_engine_campaign_binding
 ADD COLUMN admission_protocol text NOT NULL DEFAULT 'official-ce-reference-v1',
 ADD COLUMN connection_id uuid,
 ADD COLUMN provider_incarnation text,
 ADD COLUMN source_commit text,
 ADD COLUMN engine_version text,
 ADD COLUMN image_digest text,
 ADD COLUMN normalizer_version text NOT NULL DEFAULT '',
 ADD CONSTRAINT ck_annotation_source_protocol CHECK (
 admission_protocol='official-ce-reference-v1' OR
 (admission_protocol='controlled-fork-submission-v1' AND connection_id IS NOT NULL
 AND length(btrim(provider_incarnation))>0 AND source_commit ~ '^[0-9a-f]{40}$'
 AND length(btrim(engine_version))>0 AND length(btrim(normalizer_version))>0 AND image_digest ~ '^sha256:[0-9a-f]{64}$'));

CREATE TABLE annotation_source_connection_owner(
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 UNIQUE(id,workspace_id)
);
CREATE TRIGGER immutable_fact BEFORE UPDATE OR DELETE ON annotation_source_connection_owner
 FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_append_only_mutation();

CREATE TABLE annotation_source_observation(
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 connection_id uuid NOT NULL,
 campaign_id uuid NOT NULL,
 task_id uuid NOT NULL,
 input_version_id uuid NOT NULL,
 campaign_binding_id uuid NOT NULL,
 task_binding_id uuid NOT NULL,
 actor_binding_id uuid NOT NULL,
 provider_instance text NOT NULL,
 provider_incarnation text NOT NULL,
 source_kind text NOT NULL,
 external_id text NOT NULL,
 assignment_id text NOT NULL,
 external_project_id text NOT NULL,
 external_task_id text NOT NULL,
 external_annotation_id text NOT NULL,
 external_author_ref text NOT NULL,
 core_author_ref text NOT NULL,
 source_item_ref text NOT NULL,
 input_sha256 text NOT NULL,
 source_sha256 text NOT NULL,
 task_text_sha256 text NOT NULL,
 config_sha256 text NOT NULL,
 schema_sha256 text NOT NULL,
 taxonomy_sha256 text NOT NULL,
 rubric_sha256 text NOT NULL,
 renderer_sha256 text NOT NULL,
 review_policy_sha256 text NOT NULL,
 mapping_sha256 text NOT NULL,
 normalizer_version text NOT NULL,
 snapshot_sha256 text NOT NULL,
 canonical_payload_sha256 text NOT NULL,
 submission_revision bigint NOT NULL,
 fingerprint_payload bytea NOT NULL,
 fingerprint_sha256 text NOT NULL,
 snapshot bytea NOT NULL,
 canonical_payload bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,connection_id,provider_instance,provider_incarnation,source_kind,external_id),
 UNIQUE(workspace_id,connection_id,provider_instance,provider_incarnation,assignment_id,submission_revision),
 FOREIGN KEY(campaign_id) REFERENCES annotation_campaign(id),
 FOREIGN KEY(task_id) REFERENCES annotation_task(id),
 FOREIGN KEY(input_version_id) REFERENCES dataset_version(id),
 FOREIGN KEY(campaign_binding_id) REFERENCES annotation_engine_campaign_binding(id),
 FOREIGN KEY(task_binding_id) REFERENCES annotation_engine_task_binding(id),
 FOREIGN KEY(actor_binding_id) REFERENCES annotation_engine_actor_binding(id),
 CHECK(source_kind='IMMUTABLE_SUBMISSION'),
 CHECK(length(btrim(provider_instance))>0 AND provider_instance=btrim(provider_instance) AND length(btrim(provider_incarnation))>0 AND provider_incarnation=btrim(provider_incarnation) AND length(btrim(source_kind))>0 AND source_kind=btrim(source_kind) AND length(btrim(external_id))>0 AND external_id=btrim(external_id) AND length(btrim(assignment_id))>0 AND assignment_id=btrim(assignment_id) AND length(btrim(external_project_id))>0 AND external_project_id=btrim(external_project_id) AND length(btrim(external_task_id))>0 AND external_task_id=btrim(external_task_id) AND length(btrim(external_annotation_id))>0 AND external_annotation_id=btrim(external_annotation_id) AND length(btrim(external_author_ref))>0 AND external_author_ref=btrim(external_author_ref) AND length(btrim(core_author_ref))>0 AND core_author_ref=btrim(core_author_ref) AND length(btrim(source_item_ref))>0 AND source_item_ref=btrim(source_item_ref) AND input_sha256 ~ '^[0-9a-f]{64}$' AND source_sha256 ~ '^[0-9a-f]{64}$' AND task_text_sha256 ~ '^[0-9a-f]{64}$' AND config_sha256 ~ '^[0-9a-f]{64}$' AND schema_sha256 ~ '^[0-9a-f]{64}$' AND taxonomy_sha256 ~ '^[0-9a-f]{64}$' AND rubric_sha256 ~ '^[0-9a-f]{64}$' AND renderer_sha256 ~ '^[0-9a-f]{64}$' AND review_policy_sha256 ~ '^[0-9a-f]{64}$' AND mapping_sha256 ~ '^[0-9a-f]{64}$' AND length(btrim(normalizer_version))>0 AND normalizer_version=btrim(normalizer_version) AND snapshot_sha256 ~ '^[0-9a-f]{64}$' AND canonical_payload_sha256 ~ '^[0-9a-f]{64}$' AND submission_revision>0),
 CHECK(convert_from(fingerprint_payload,'UTF8')::jsonb=jsonb_build_object('workspaceID', workspace_id::text,'connectionID', connection_id::text,'campaignID', campaign_id::text,'taskID', task_id::text,'inputVersionID', input_version_id::text,'campaignBindingID', campaign_binding_id::text,'taskBindingID', task_binding_id::text,'actorBindingID', actor_binding_id::text,'providerInstance', provider_instance,'providerIncarnation', provider_incarnation,'sourceKind', source_kind,'externalID', external_id,'assignmentID', assignment_id,'externalProjectID', external_project_id,'externalTaskID', external_task_id,'externalAnnotationID', external_annotation_id,'externalAuthorRef', external_author_ref,'coreAuthorRef', core_author_ref,'sourceItemRef', source_item_ref,'inputSHA256', input_sha256,'sourceSHA256', source_sha256,'taskTextSHA256', task_text_sha256,'configSHA256', config_sha256,'schemaSHA256', schema_sha256,'taxonomySHA256', taxonomy_sha256,'rubricSHA256', rubric_sha256,'rendererSHA256', renderer_sha256,'reviewPolicySHA256', review_policy_sha256,'mappingSHA256', mapping_sha256,'normalizerVersion', normalizer_version,'snapshotSHA256', snapshot_sha256,'canonicalPayloadSHA256', canonical_payload_sha256,'submissionRevision', submission_revision)),
 CHECK(encode(digest(fingerprint_payload,'sha256'),'hex')=fingerprint_sha256),
 CHECK(encode(digest(snapshot,'sha256'),'hex')=snapshot_sha256),
 CHECK(convert_from(snapshot,'UTF8')::jsonb IS NOT NULL),
 CHECK(convert_from(canonical_payload,'UTF8')::jsonb IS NOT NULL),
 FOREIGN KEY(connection_id,workspace_id) REFERENCES annotation_source_connection_owner(id,workspace_id),
 CHECK(encode(digest(canonical_payload,'sha256'),'hex')=canonical_payload_sha256)
);
CREATE TABLE annotation_source_result_binding(
 result_id uuid PRIMARY KEY REFERENCES annotation_result(id),
 source_id uuid NOT NULL UNIQUE REFERENCES annotation_source_observation(id),
 fingerprint_sha256 text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE annotation_source_receipt(
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 campaign_id uuid NOT NULL REFERENCES annotation_campaign(id),
 task_id uuid NOT NULL REFERENCES annotation_task(id),
 physical_attempt_id uuid REFERENCES annotation_engine_attempt(id),
 source_id uuid REFERENCES annotation_source_observation(id),
 incoming_fingerprint_sha256 text NOT NULL,
 snapshot bytea NOT NULL,
 canonical_payload bytea NOT NULL,
 disposition text NOT NULL CHECK(disposition IN ('OBSERVED','LATE','QUARANTINED','CONFLICT')),
 observed_assignment_version bigint NOT NULL CHECK(observed_assignment_version>0),
 observed_at timestamptz NOT NULL,
 assignment_version_semantics text NOT NULL CHECK(assignment_version_semantics='OBSERVED_CURRENT'),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE annotation_submission_batch(
 campaign_id uuid PRIMARY KEY REFERENCES annotation_campaign(id),
 workspace_id uuid NOT NULL,
 expected_task_ids uuid[] NOT NULL,
 expected_assignment_ids text[] NOT NULL,
 expected_revisions bigint[] NOT NULL,
 page_budget integer NOT NULL CHECK(page_budget BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(cardinality(expected_task_ids)>0
 AND cardinality(expected_task_ids)=cardinality(expected_assignment_ids)
 AND cardinality(expected_task_ids)=cardinality(expected_revisions))
);
CREATE TABLE annotation_submission_batch_receipt(
 id uuid PRIMARY KEY,
 campaign_id uuid NOT NULL REFERENCES annotation_submission_batch(campaign_id),
 sequence bigint NOT NULL CHECK(sequence>0),
 outcome text NOT NULL CHECK(outcome IN ('UNRESOLVED','COMPLETE')),
 scan_started_receipt_id uuid REFERENCES annotation_submission_batch_receipt(id),
 CHECK((outcome='COMPLETE')=(scan_started_receipt_id IS NOT NULL)),
 source_ids uuid[] NOT NULL,
 fingerprints text[] NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(campaign_id,sequence),
 CHECK(cardinality(source_ids)=cardinality(fingerprints))
);

CREATE FUNCTION annotation_validate_source_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c annotation_campaign; t annotation_task; b annotation_engine_campaign_binding;
 tb annotation_engine_task_binding; ab annotation_engine_actor_binding;
BEGIN
 SELECT * INTO c FROM annotation_campaign WHERE id=NEW.campaign_id FOR UPDATE;
 SELECT * INTO t FROM annotation_task WHERE id=NEW.task_id FOR UPDATE;
 SELECT * INTO b FROM annotation_engine_campaign_binding WHERE id=NEW.campaign_binding_id;
 SELECT * INTO tb FROM annotation_engine_task_binding WHERE id=NEW.task_binding_id;
 SELECT * INTO ab FROM annotation_engine_actor_binding WHERE id=NEW.actor_binding_id;
 IF c.workspace_id IS DISTINCT FROM NEW.workspace_id OR t.campaign_id IS DISTINCT FROM c.id
 OR t.workspace_id IS DISTINCT FROM NEW.workspace_id OR b.campaign_id IS DISTINCT FROM c.id
 OR b.admission_protocol IS DISTINCT FROM 'controlled-fork-submission-v1'
 OR b.connection_id IS DISTINCT FROM NEW.connection_id
 OR b.provider_instance_ref IS DISTINCT FROM NEW.provider_instance
 OR b.provider_incarnation IS DISTINCT FROM NEW.provider_incarnation
 OR b.external_project_id IS DISTINCT FROM NEW.external_project_id
 OR b.normalizer_version IS DISTINCT FROM NEW.normalizer_version OR b.config_sha256 IS DISTINCT FROM NEW.config_sha256
 OR tb.campaign_binding_id IS DISTINCT FROM b.id OR tb.task_id IS DISTINCT FROM t.id
 OR tb.external_task_id IS DISTINCT FROM NEW.external_task_id
 OR ab.workspace_id IS DISTINCT FROM NEW.workspace_id OR ab.provider IS DISTINCT FROM b.provider
 OR ab.provider_instance_ref IS DISTINCT FROM b.provider_instance_ref
 OR ab.external_actor_ref IS DISTINCT FROM NEW.external_author_ref
 OR ab.core_actor_ref IS DISTINCT FROM NEW.core_author_ref
 OR t.primary_annotator_ref IS DISTINCT FROM NEW.core_author_ref
 OR c.input_dataset_version_id IS DISTINCT FROM NEW.input_version_id
 OR c.input_checksum_sha256 IS DISTINCT FROM NEW.input_sha256
 OR t.source_item_ref IS DISTINCT FROM NEW.source_item_ref
 OR t.source_content_sha256 IS DISTINCT FROM NEW.source_sha256
 OR t.task_text_sha256 IS DISTINCT FROM NEW.task_text_sha256
 OR c.schema_content_sha256 IS DISTINCT FROM NEW.schema_sha256
 OR c.taxonomy_content_sha256 IS DISTINCT FROM NEW.taxonomy_sha256
 OR c.rubric_content_sha256 IS DISTINCT FROM NEW.rubric_sha256
 OR c.renderer_content_sha256 IS DISTINCT FROM NEW.renderer_sha256
 OR c.review_policy_content_sha256 IS DISTINCT FROM NEW.review_policy_sha256 THEN
 RAISE EXCEPTION 'immutable source crosses frozen Core context'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER annotation_source_validate BEFORE INSERT ON annotation_source_observation
 FOR EACH ROW EXECUTE FUNCTION annotation_validate_source_insert();

CREATE FUNCTION annotation_validate_source_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r annotation_result; s annotation_source_observation;
BEGIN
 -- Fixed parent-first locking even for direct SQL binding writes.
 PERFORM c.id FROM annotation_campaign c JOIN annotation_result rr ON rr.campaign_id=c.id
 WHERE rr.id=NEW.result_id FOR UPDATE OF c;
 SELECT * INTO r FROM annotation_result WHERE id=NEW.result_id;
 PERFORM id FROM annotation_task WHERE id=r.task_id FOR UPDATE;
 SELECT * INTO s FROM annotation_source_observation WHERE id=NEW.source_id;
 IF r.corrected_from_result_id IS NOT NULL OR r.workspace_id IS DISTINCT FROM s.workspace_id
 OR r.campaign_id IS DISTINCT FROM s.campaign_id OR r.task_id IS DISTINCT FROM s.task_id
 OR r.author_ref IS DISTINCT FROM s.core_author_ref
 OR r.provider_binding_ref IS DISTINCT FROM s.campaign_binding_id::text
 OR r.external_task_id IS DISTINCT FROM s.external_task_id
 OR r.external_annotation_id IS DISTINCT FROM s.external_annotation_id
 OR r.external_revision IS DISTINCT FROM
 ('submission/'||s.external_id||'/assignment/'||s.assignment_id||'/revision/'||s.submission_revision)
 OR r.canonical_payload IS DISTINCT FROM s.canonical_payload
 OR r.canonical_payload_sha256 IS DISTINCT FROM s.canonical_payload_sha256
 OR r.normalizer_version IS DISTINCT FROM s.normalizer_version
 OR NEW.fingerprint_sha256 IS DISTINCT FROM s.fingerprint_sha256
 OR NOT EXISTS(SELECT 1 FROM annotation_campaign WHERE id=r.campaign_id AND status='ACTIVE')
 OR EXISTS(SELECT 1 FROM annotation_review_decision WHERE task_id=r.task_id) THEN
 RAISE EXCEPTION 'invalid exact SourceResultBinding'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER annotation_source_binding_validate BEFORE INSERT ON annotation_source_result_binding
 FOR EACH ROW EXECUTE FUNCTION annotation_validate_source_binding();

CREATE FUNCTION annotation_require_source_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM annotation_engine_campaign_binding b
 WHERE b.campaign_id=NEW.campaign_id AND b.admission_protocol='controlled-fork-submission-v1')
 AND NEW.corrected_from_result_id IS NULL
 AND NOT EXISTS(SELECT 1 FROM annotation_source_result_binding WHERE result_id=NEW.id) THEN
 RAISE EXCEPTION 'controlled Result requires same-transaction SourceResultBinding'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER annotation_result_source_required AFTER INSERT ON annotation_result
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION annotation_require_source_binding();

CREATE FUNCTION annotation_validate_batch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM annotation_campaign WHERE id=NEW.campaign_id FOR UPDATE;
 IF NOT EXISTS(SELECT 1 FROM annotation_campaign WHERE id=NEW.campaign_id
 AND workspace_id=NEW.workspace_id AND status='ACTIVE')
 OR NOT EXISTS(SELECT 1 FROM annotation_engine_campaign_binding WHERE campaign_id=NEW.campaign_id
 AND admission_protocol='controlled-fork-submission-v1')
 OR EXISTS(SELECT 1 FROM annotation_result WHERE campaign_id=NEW.campaign_id)
 OR EXISTS(SELECT 1 FROM unnest(NEW.expected_task_ids,NEW.expected_assignment_ids,NEW.expected_revisions)
 AS e(task_id,assignment_id,revision) WHERE task_id IS NULL OR revision IS NULL OR revision<1 OR assignment_id IS NULL OR length(btrim(assignment_id))=0 OR assignment_id<>btrim(assignment_id))
 OR EXISTS(SELECT 1 FROM unnest(NEW.expected_assignment_ids,NEW.expected_revisions) AS e(a,r)
 GROUP BY a,r HAVING count(*)>1)
 OR EXISTS(SELECT id FROM annotation_task WHERE campaign_id=NEW.campaign_id
 EXCEPT SELECT unnest(NEW.expected_task_ids))
 OR EXISTS(SELECT unnest(NEW.expected_task_ids) EXCEPT
 SELECT id FROM annotation_task WHERE campaign_id=NEW.campaign_id) THEN
 RAISE EXCEPTION 'invalid frozen finite Submission batch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER annotation_batch_validate BEFORE INSERT ON annotation_submission_batch
 FOR EACH ROW EXECUTE FUNCTION annotation_validate_batch();

CREATE FUNCTION annotation_require_complete_batch() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cid uuid;
BEGIN
 cid:=NEW.campaign_id;
 PERFORM id FROM annotation_campaign WHERE id=cid FOR UPDATE;
 IF EXISTS(SELECT 1 FROM annotation_engine_campaign_binding WHERE campaign_id=cid
 AND admission_protocol='controlled-fork-submission-v1')
 AND NOT EXISTS(SELECT 1 FROM annotation_submission_batch_receipt WHERE campaign_id=cid
 AND outcome='COMPLETE' ORDER BY sequence DESC LIMIT 1)
 THEN RAISE EXCEPTION 'controlled batch has no complete receipt'; END IF;
 IF EXISTS(SELECT 1 FROM annotation_engine_campaign_binding WHERE campaign_id=cid
 AND admission_protocol='controlled-fork-submission-v1')
 AND (SELECT outcome FROM annotation_submission_batch_receipt WHERE campaign_id=cid
 ORDER BY sequence DESC LIMIT 1) IS DISTINCT FROM 'COMPLETE'
 THEN RAISE EXCEPTION 'controlled batch is unresolved'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER annotation_review_batch_complete BEFORE INSERT ON annotation_review_decision
 FOR EACH ROW EXECUTE FUNCTION annotation_require_complete_batch();
CREATE TRIGGER annotation_snapshot_batch_complete BEFORE INSERT ON annotation_snapshot
 FOR EACH ROW EXECUTE FUNCTION annotation_require_complete_batch();

DO $$
DECLARE tbl text;
BEGIN
 FOREACH tbl IN ARRAY ARRAY['annotation_source_observation','annotation_source_result_binding',
 'annotation_source_receipt','annotation_submission_batch','annotation_submission_batch_receipt'] LOOP
 EXECUTE format('CREATE TRIGGER immutable_fact BEFORE UPDATE OR DELETE ON %I
 FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_append_only_mutation()',tbl);
 END LOOP;
END $$;

-- Completion receipts cannot be self-declared, and binding mode cannot retrofit
-- existing mutable/CE facts into controlled history.
CREATE FUNCTION annotation_validate_controlled_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.admission_protocol='controlled-fork-submission-v1' THEN
 PERFORM id FROM annotation_campaign WHERE id=NEW.campaign_id FOR UPDATE;
 INSERT INTO annotation_source_connection_owner(id,workspace_id) VALUES(NEW.connection_id,NEW.workspace_id) ON CONFLICT(id) DO NOTHING;
 IF NOT EXISTS(SELECT 1 FROM annotation_source_connection_owner WHERE id=NEW.connection_id AND workspace_id=NEW.workspace_id) THEN
 RAISE EXCEPTION 'controlled source connection is owned by another workspace'; END IF;
 IF EXISTS(SELECT 1 FROM annotation_result WHERE campaign_id=NEW.campaign_id)
 OR EXISTS(SELECT 1 FROM annotation_review_decision WHERE campaign_id=NEW.campaign_id)
 OR EXISTS(SELECT 1 FROM annotation_snapshot WHERE campaign_id=NEW.campaign_id) THEN
 RAISE EXCEPTION 'controlled mode must be frozen before Result admission'; END IF;
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER annotation_controlled_binding_validate BEFORE INSERT ON annotation_engine_campaign_binding
 FOR EACH ROW EXECUTE FUNCTION annotation_validate_controlled_binding();

CREATE FUNCTION annotation_validate_batch_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE batch annotation_submission_batch;
BEGIN
 PERFORM id FROM annotation_campaign WHERE id=NEW.campaign_id FOR UPDATE;
 SELECT * INTO batch FROM annotation_submission_batch WHERE campaign_id=NEW.campaign_id;
 IF NEW.outcome='COMPLETE' THEN
 IF NEW.scan_started_receipt_id IS DISTINCT FROM (SELECT id FROM annotation_submission_batch_receipt WHERE campaign_id=NEW.campaign_id ORDER BY sequence DESC LIMIT 1)
 OR NOT EXISTS(SELECT 1 FROM annotation_submission_batch_receipt WHERE id=NEW.scan_started_receipt_id AND campaign_id=NEW.campaign_id AND outcome='UNRESOLVED')
 OR cardinality(NEW.source_ids)<>cardinality(batch.expected_task_ids)
 OR EXISTS(SELECT 1 FROM unnest(NEW.source_ids) AS ids(id) GROUP BY id HAVING count(*)>1)
 OR EXISTS(
 SELECT 1 FROM unnest(batch.expected_task_ids,batch.expected_assignment_ids,batch.expected_revisions) AS e(t,a,r)
 WHERE NOT EXISTS(SELECT 1 FROM annotation_source_observation s
 JOIN annotation_source_result_binding b ON b.source_id=s.id
 WHERE s.campaign_id=NEW.campaign_id AND s.task_id=e.t AND s.assignment_id=e.a AND s.submission_revision=e.r
 AND s.id=ANY(NEW.source_ids)))
 OR EXISTS(
 SELECT 1 FROM unnest(NEW.source_ids,NEW.fingerprints) AS p(id,hash)
 WHERE NOT EXISTS(SELECT 1 FROM annotation_source_observation s
 JOIN annotation_source_result_binding b ON b.source_id=s.id
 WHERE s.id=p.id AND s.campaign_id=NEW.campaign_id AND s.fingerprint_sha256=p.hash
 AND b.fingerprint_sha256=p.hash))
 OR (SELECT count(*) FROM annotation_result WHERE campaign_id=NEW.campaign_id AND corrected_from_result_id IS NULL)
 <>cardinality(batch.expected_task_ids)
 THEN RAISE EXCEPTION 'Submission completion receipt does not cover exact accepted batch'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER annotation_batch_receipt_validate BEFORE INSERT ON annotation_submission_batch_receipt
 FOR EACH ROW EXECUTE FUNCTION annotation_validate_batch_receipt();

CREATE FUNCTION annotation_source_closure_json(cid uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 WITH RECURSIVE chain AS(
 SELECT id result_id,id node_id,corrected_from_result_id parent_id,
 workspace_id,campaign_id,task_id,ARRAY[id] path FROM annotation_result WHERE campaign_id=cid
 UNION ALL
 SELECT ch.result_id,p.id,p.corrected_from_result_id,ch.workspace_id,ch.campaign_id,ch.task_id,ch.path||p.id
 FROM chain ch JOIN annotation_result p ON p.id=ch.parent_id
 WHERE p.workspace_id=ch.workspace_id AND p.campaign_id=ch.campaign_id AND p.task_id=ch.task_id
 AND NOT p.id=ANY(ch.path)
 ), roots AS(
 SELECT ch.result_id,ch.node_id,s.id source_id,s.fingerprint_sha256,s.snapshot_sha256
 FROM chain ch JOIN annotation_source_result_binding b ON b.result_id=ch.node_id
 JOIN annotation_source_observation s ON s.id=b.source_id
 JOIN annotation_result r ON r.id=ch.node_id
 WHERE ch.parent_id IS NULL AND b.fingerprint_sha256=s.fingerprint_sha256
 AND s.workspace_id=r.workspace_id AND s.campaign_id=r.campaign_id AND s.task_id=r.task_id
 AND s.canonical_payload=r.canonical_payload AND s.canonical_payload_sha256=r.canonical_payload_sha256
 AND s.core_author_ref=r.author_ref AND s.normalizer_version=r.normalizer_version
 AND s.campaign_binding_id::text=r.provider_binding_ref
 AND s.external_task_id=r.external_task_id AND s.external_annotation_id=r.external_annotation_id
 AND r.external_revision='submission/'||s.external_id||'/assignment/'||s.assignment_id||'/revision/'||s.submission_revision
 AND encode(digest(s.snapshot,'sha256'),'hex')=s.snapshot_sha256
 AND encode(digest(s.fingerprint_payload,'sha256'),'hex')=s.fingerprint_sha256
 AND encode(digest(s.canonical_payload,'sha256'),'hex')=s.canonical_payload_sha256
 )
 SELECT COALESCE(jsonb_agg(jsonb_build_object('resultId',result_id::text,
 'originalResultId',node_id::text,'sourceId',source_id::text,
 'fingerprintSha256',fingerprint_sha256,'snapshotSha256',snapshot_sha256) ORDER BY result_id::text),'[]'::jsonb) FROM roots
$$;
CREATE FUNCTION annotation_snapshot_source_freeze() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE closure jsonb;
BEGIN
 IF OLD.status='BUILDING' AND NEW.status='FINALIZED' AND EXISTS(
 SELECT 1 FROM annotation_engine_campaign_binding WHERE campaign_id=NEW.campaign_id
 AND admission_protocol='controlled-fork-submission-v1') THEN
 PERFORM id FROM annotation_campaign WHERE id=NEW.campaign_id FOR UPDATE;
 closure:=annotation_source_closure_json(NEW.campaign_id);
 IF NEW.manifest->>'sourceProtocol' IS DISTINCT FROM 'controlled-fork-submission-v1'
 OR NEW.manifest->'sourceClosure' IS DISTINCT FROM closure
 OR jsonb_array_length(closure)<>(SELECT count(*) FROM annotation_result WHERE campaign_id=NEW.campaign_id)
 OR EXISTS(SELECT 1 FROM annotation_result WHERE campaign_id=NEW.campaign_id
 AND encode(digest(canonical_payload,'sha256'),'hex')<>canonical_payload_sha256)
 OR NOT EXISTS(SELECT 1 FROM annotation_submission_batch_receipt
 WHERE id=(NEW.manifest->>'submissionBatchReceiptId')::uuid AND campaign_id=NEW.campaign_id AND outcome='COMPLETE')
 THEN RAISE EXCEPTION 'controlled Snapshot source closure is incomplete'; END IF;
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER annotation_snapshot_source_freeze BEFORE UPDATE OF status ON annotation_snapshot
 FOR EACH ROW EXECUTE FUNCTION annotation_snapshot_source_freeze();


CREATE FUNCTION annotation_validate_source_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM annotation_campaign WHERE id=NEW.campaign_id FOR UPDATE;
 IF NOT EXISTS(SELECT 1 FROM annotation_task t JOIN annotation_campaign c ON c.id=t.campaign_id
 WHERE t.id=NEW.task_id AND c.id=NEW.campaign_id AND t.workspace_id=NEW.workspace_id AND c.workspace_id=NEW.workspace_id)
 OR NEW.physical_attempt_id IS NULL
 OR NOT EXISTS(SELECT 1 FROM annotation_engine_attempt a JOIN annotation_engine_operation o ON o.id=a.operation_id
 WHERE a.id=NEW.physical_attempt_id AND a.workspace_id=NEW.workspace_id AND o.campaign_id=NEW.campaign_id)
 OR NEW.incoming_fingerprint_sha256 !~ '^[0-9a-f]{64}$'
 OR (NEW.disposition='CONFLICT') IS DISTINCT FROM (NEW.source_id IS NULL)
 OR (NEW.source_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM annotation_source_observation s
 WHERE s.id=NEW.source_id AND s.workspace_id=NEW.workspace_id AND s.campaign_id=NEW.campaign_id AND s.task_id=NEW.task_id
 AND s.fingerprint_sha256=NEW.incoming_fingerprint_sha256 AND s.snapshot=NEW.snapshot AND s.canonical_payload=NEW.canonical_payload))
 THEN RAISE EXCEPTION 'source receipt crosses physical attempt or source context'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER annotation_source_receipt_validate BEFORE INSERT ON annotation_source_receipt
 FOR EACH ROW EXECUTE FUNCTION annotation_validate_source_receipt();
