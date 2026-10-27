-- Synthetic historical read fixtures only, in a new disposable database.
-- No quality engine, processing, certification, delivery or model is invoked.
BEGIN;
INSERT INTO dataset(id, workspace_id, code, name, dataset_type, metadata) VALUES
('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','11111111-1111-4111-8111-111111111111','AGENT_Q1_CORE','Core只读联调 · 企业基础表','RAW','{"syntheticAcceptance":true}'),
('a2100001-a101-4101-8101-000000000001','12121212-1212-4212-8212-121212121212','AGENT_Q1_FOREIGN','其他空间 · 隔离样本','RAW','{"syntheticAcceptance":true}');
INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_algorithm,checksum_value,row_count,created_at,ready_at,metadata) VALUES
('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',1,'READY','s3://agent-q1-live/synthetic.csv','SHA256',encode(digest('synthetic-acceptance-only','sha256'),'hex'),26,'2026-10-20T00:00:00Z','2026-10-20T00:00:00Z','{"syntheticAcceptance":true,"privateSentinel":"q1-storage-private"}'),
('a2100002-a101-4101-8101-000000000002','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',2,'READY','s3://agent-q1-live/synthetic-v2.csv','SHA256',encode(digest('synthetic-v2-only','sha256'),'hex'),30,'2026-10-22T00:00:00Z','2026-10-22T00:00:00Z','{"syntheticAcceptance":true}'),
('a2100002-a101-4101-8101-000000000003','a2100001-a101-4101-8101-000000000001',1,'CREATED',NULL,NULL,NULL,NULL,'2026-10-20T00:00:00Z',NULL,'{"syntheticAcceptance":true}');
UPDATE dataset SET current_version_id='a2100002-a101-4101-8101-000000000002' WHERE id='aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';

-- Findings must precede their immutable parent, under the existing deferred FK.
INSERT INTO quality_finding(id,result_id,rule_id,dimension,severity,status,observed,message,created_at)
SELECT ('a2100004-a101-4101-8101-'||lpad(i::text,12,'0'))::uuid,
  'cccccccc-cccc-4ccc-8ccc-cccccccccccc', 'Q1-SYNTHETIC-'||lpad(i::text,2,'0'),
  'COMPLETENESS', CASE WHEN i=1 THEN 'CRITICAL' ELSE 'WARNING' END, 'FAIL',
  jsonb_build_object('affectedCount',1,'sample',jsonb_build_array(jsonb_build_object('row',i))),
  '持久化合成发现项 '||i, '2026-10-20T01:00:00Z' FROM generate_series(1,26) i;

INSERT INTO quality_result(id,workspace_id,dataset_version_id,rule_set_ref,rule_set_version,gate_decision,metrics,
 rule_set_content,rule_set_content_sha256,evaluator_name,evaluator_version,created_at)
VALUES ('cccccccc-cccc-4ccc-8ccc-cccccccccccc','11111111-1111-4111-8111-111111111111','bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
 'fixture/agent-q1-historical.yaml','1','FAIL',
 '{"syntheticAcceptance":true,"dimensions":{"COMPLETENESS":{"dimension":"COMPLETENESS","status":"FAIL","ruleCount":26,"evaluatedCount":26,"failedCount":26}}}',
 'synthetic-acceptance-only',encode(digest('synthetic-acceptance-only','sha256'),'hex'),'agent-q1-synthetic-fixture','1','2026-10-20T01:00:00Z');
INSERT INTO quality_result(id,workspace_id,dataset_version_id,rule_set_ref,rule_set_version,gate_decision,metrics,
 rule_set_content,rule_set_content_sha256,evaluator_name,evaluator_version,created_at)
SELECT CASE WHEN i=25 THEN '47474747-4747-4747-8747-474747474747'::uuid ELSE ('a2100003-a101-4101-8101-'||lpad(i::text,12,'0'))::uuid END,
 '11111111-1111-4111-8111-111111111111','bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
 'fixture/agent-q1-history-'||i||'.yaml','1','PASS',
 '{"syntheticAcceptance":true,"dimensions":{"COMPLETENESS":{"dimension":"COMPLETENESS","status":"PASS","ruleCount":1,"evaluatedCount":1,"failedCount":0}}}'::jsonb,
 'synthetic-acceptance-only',encode(digest('synthetic-acceptance-only','sha256'),'hex'),'agent-q1-synthetic-fixture','1',
 '2026-10-21T00:00:00Z'::timestamptz + i * interval '1 minute' FROM generate_series(1,25) i;
INSERT INTO quality_result(id,workspace_id,dataset_version_id,rule_set_ref,rule_set_version,gate_decision,metrics,
 rule_set_content,rule_set_content_sha256,evaluator_name,evaluator_version,created_at)
VALUES ('a2100003-a101-4101-8101-000000000101','11111111-1111-4111-8111-111111111111','a2100002-a101-4101-8101-000000000002',
 'fixture/agent-q1-v2.yaml','1','PASS','{"syntheticAcceptance":true,"dimensions":{}}',
 'synthetic-acceptance-only',encode(digest('synthetic-acceptance-only','sha256'),'hex'),'agent-q1-synthetic-fixture','1','2026-10-22T01:00:00Z');
INSERT INTO quality_assessment_attempt(id,workspace_id,dataset_version_id,rule_set_ref,started_at,lease_expires_at,engine_name,engine_version)
VALUES ('66666666-6666-4666-8666-666666666666','11111111-1111-4111-8111-111111111111','bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
 'fixture/agent-q1-historical.yaml','2026-10-20T00:59:00Z','2026-10-20T01:01:00Z','agent-q1-synthetic-fixture','1');
INSERT INTO quality_assessment_attempt_outcome(id,attempt_id,assessment_id,outcome,occurred_at)
VALUES ('a2100005-a101-4101-8101-000000000001','66666666-6666-4666-8666-666666666666','cccccccc-cccc-4ccc-8ccc-cccccccccccc','SUCCEEDED','2026-10-20T01:00:00Z');
INSERT INTO audit_event(id,workspace_id,actor_type,action,object_type,object_id,metadata,occurred_at)
VALUES ('a2100006-a101-4101-8101-000000000001','11111111-1111-4111-8111-111111111111','SYSTEM','QUALITY_CHECK_COMPLETED','QUALITY_RESULT','cccccccc-cccc-4ccc-8ccc-cccccccccccc','{"syntheticAcceptance":true}','2026-10-20T01:00:00Z');
-- Synthetic reference; hash generated by Core evidence.ComputeHash over this record.
INSERT INTO evidence(id,workspace_id,evidence_type,title,source_type,source_id,hash_algorithm,hash_value,metadata,created_at)
VALUES ('a2100007-a101-4101-8101-000000000001','11111111-1111-4111-8111-111111111111','QUALITY_RESULT','Q1合成验收证据引用','QUALITY_RESULT','cccccccc-cccc-4ccc-8ccc-cccccccccccc',
 'SHA256-EVIDENCE-V2','c390e595f3ef3a813c40006c1395e91c4a229d7168b5a4bb3c3252ca66ee4a44','{"syntheticAcceptance":true}','2026-10-20T01:00:00Z');
INSERT INTO evidence_relation(evidence_id,object_type,object_id,relation_type,created_at)
VALUES ('a2100007-a101-4101-8101-000000000001','QUALITY_RESULT','cccccccc-cccc-4ccc-8ccc-cccccccccccc','EVIDENCE_FOR','2026-10-20T01:00:00Z');
COMMIT;

-- Core runtime has no DML, DDL or sequence privileges. Bootstrap is now over.
CREATE ROLE agent_q1_reader LOGIN PASSWORD 'agent-q1-reader-only';
ALTER ROLE agent_q1_reader SET default_transaction_read_only=on;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE agent_q1_live TO agent_q1_reader;
GRANT USAGE ON SCHEMA public TO agent_q1_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO agent_q1_reader;
