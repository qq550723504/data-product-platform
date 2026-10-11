-- Historical cases/attempts must not be silently removed, including via down.
DO $$ BEGIN RAISE EXCEPTION 'quality remediation history cannot be destructively downgraded'; END; $$;
