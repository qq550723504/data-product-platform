-- Never erase or invalidate incurred maintenance invocation history.
DO $$ BEGIN RAISE EXCEPTION 'checkpoint read facts are append-only; destructive down is unsupported'; END $$;
