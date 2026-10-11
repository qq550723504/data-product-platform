-- Removing the stable identity guard would reopen cross-workspace takeover on rename.
DO $$ BEGIN
    RAISE EXCEPTION '000053 protects metadata binding ownership; rollback requires an explicit database rebuild';
END; $$;
