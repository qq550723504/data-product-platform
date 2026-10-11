DO $$ BEGIN RAISE EXCEPTION 'lifecycle denial/reconciliation history must not be destroyed'; END $$;
