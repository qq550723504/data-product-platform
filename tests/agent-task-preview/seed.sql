-- Run once against the owned disposable agent_q2_test database only.
BEGIN;
INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES
('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa','11111111-1111-4111-8111-111111111111','AGENT-Q2-SOURCE','Q2 合成验收来源','RAW');
INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_algorithm,checksum_value,ready_at) VALUES
('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',1,'READY','s3://agent-q2-synthetic/source.csv','SHA256',repeat('a',64),now());
CREATE ROLE agent_q2_planner LOGIN PASSWORD 'agent-q2-planner-only';
GRANT CONNECT ON DATABASE agent_q2_test TO agent_q2_planner;
GRANT USAGE ON SCHEMA public TO agent_q2_planner;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO agent_q2_planner;
-- PostgreSQL FOR UPDATE requires UPDATE privilege. These locks do not grant
-- the host any Quality/Execution commands; it registers Agent routes only.
GRANT UPDATE ON agent_task,agent_plan_revision TO agent_q2_planner;
-- Owner preparation locks the immutable case. Its trigger rejects all updates.
GRANT UPDATE(id) ON quality_remediation_case TO agent_q2_planner;
GRANT INSERT ON agent_task,agent_plan_revision,agent_plan_input,agent_plan_action,
 agent_approval,agent_action_request,agent_action_disposition,agent_command_receipt,
 audit_event,evidence,evidence_relation,outbox_event TO agent_q2_planner;
COMMIT;
