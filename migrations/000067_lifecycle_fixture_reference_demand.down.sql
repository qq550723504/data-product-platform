-- Reference demands/releases explain retained history; rollback cannot erase it.
DO $$ BEGIN RAISE EXCEPTION 'lifecycle reference demand history cannot be rolled back'; END $$;
