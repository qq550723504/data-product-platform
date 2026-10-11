-- This migration owns append-only request/attempt/receipt and cost history.
-- A destructive rollback must never erase even an unsuccessful invocation.
DO $$ BEGIN
 RAISE EXCEPTION '000051 records ingestion history; destructive rollback is unsupported';
END; $$;
