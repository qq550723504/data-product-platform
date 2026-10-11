-- Engine invocation and outcome history must survive rollback.
DO $$ BEGIN
 RAISE EXCEPTION '000052 records ingestion invocation outcomes; destructive rollback is unsupported';
END; $$;
