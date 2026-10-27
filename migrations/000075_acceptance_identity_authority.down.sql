DO $$ BEGIN RAISE EXCEPTION 'Acceptance identity authority/history and shared fences require an explicit forward migration; destructive down is unsupported'; END; $$;
