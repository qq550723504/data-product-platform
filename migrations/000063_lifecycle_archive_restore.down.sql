DO $$ BEGIN RAISE EXCEPTION 'archive/restore history cannot be removed by destructive down'; END $$;
