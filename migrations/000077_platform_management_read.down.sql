DO $$ BEGIN RAISE EXCEPTION 'Platform management authority history cannot be weakened by down migration'; END $$;
