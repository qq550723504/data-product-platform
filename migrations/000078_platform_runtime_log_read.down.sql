DO $$ BEGIN RAISE EXCEPTION 'Runtime log authority history cannot be weakened by down migration'; END $$;
