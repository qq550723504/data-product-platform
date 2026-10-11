DO $$ BEGIN RAISE EXCEPTION 'reconciliation requests and observations cannot be removed by destructive down'; END $$;
