-- Never remove accepted planning/approval history. Lock before checking so a
-- concurrent first command cannot cross the down-migration precheck.
LOCK TABLE agent_task,agent_plan_revision,agent_plan_input,agent_plan_action,agent_approval,agent_action_request,agent_action_disposition,agent_command_receipt IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM agent_task) OR EXISTS(SELECT 1 FROM agent_plan_revision) OR EXISTS(SELECT 1 FROM agent_command_receipt)
  THEN RAISE EXCEPTION 'Agent planning history exists; down refused'; END IF;
END $$;
DROP TABLE agent_command_receipt,agent_action_disposition,agent_action_request,agent_approval,agent_plan_input,agent_plan_action;
ALTER TABLE agent_task DROP CONSTRAINT agent_task_current_plan;
DROP TABLE agent_plan_revision,agent_task;
DROP FUNCTION guard_agent_fact(),guard_agent_task(),agent_plan_must_be_frozen(),guard_agent_plan(),guard_agent_plan_member(),agent_plan_content(uuid);
