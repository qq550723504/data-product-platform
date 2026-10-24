-- No business/audit facts, but active sessions must drain before rollback.
-- The migration runner executes this file in one transaction. Serialize session
-- writers BEFORE checking; no INSERT can cross the empty/expired-state check.
LOCK TABLE platform_browser_transaction, platform_browser_session IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform_browser_transaction WHERE expires_at>statement_timestamp())
 OR EXISTS(SELECT 1 FROM platform_browser_session WHERE expires_at>statement_timestamp())
 THEN RAISE EXCEPTION 'active browser sessions must be revoked/drained before rollback'; END IF;
END $$;
DROP TABLE platform_browser_session;
DROP TABLE platform_browser_transaction;
