DO $$ BEGIN
 RAISE EXCEPTION 'workspace object identities and physical call history cannot be rolled back';
END; $$;
