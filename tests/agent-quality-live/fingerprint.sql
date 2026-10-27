-- Snapshot every public table without persisting a business fact or dumping rows.
BEGIN;
CREATE TEMP TABLE q1_fingerprint(table_name text, row_count bigint, content_sha256 text);
DO $$ DECLARE item record; BEGIN
  FOR item IN SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename LOOP
    EXECUTE format('INSERT INTO q1_fingerprint SELECT %L,count(*),encode(digest(coalesce(string_agg(to_jsonb(t)::text,E''\n'' ORDER BY to_jsonb(t)::text),''''),''sha256''),''hex'') FROM public.%I t', item.tablename,item.tablename);
  END LOOP;
END $$;
SELECT jsonb_pretty(jsonb_agg(to_jsonb(f) ORDER BY table_name)) FROM q1_fingerprint f;
ROLLBACK;
