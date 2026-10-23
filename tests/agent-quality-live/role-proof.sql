SELECT jsonb_build_object(
 'user',current_user,
 'readOnly',current_setting('default_transaction_read_only'),
 'superuser',(SELECT rolsuper FROM pg_roles WHERE rolname=current_user),
 'publicSchemaCreate',has_schema_privilege(current_user,'public','CREATE'),
 'publicTables',(SELECT count(*) FROM pg_tables WHERE schemaname='public'),
 'readableTables',(SELECT count(*) FROM pg_tables WHERE schemaname='public'
    AND has_table_privilege(current_user,format('%I.%I',schemaname,tablename),'SELECT')),
 'writableTables',(SELECT count(*) FROM pg_tables WHERE schemaname='public'
    AND (has_table_privilege(current_user,format('%I.%I',schemaname,tablename),'INSERT')
      OR has_table_privilege(current_user,format('%I.%I',schemaname,tablename),'UPDATE')
      OR has_table_privilege(current_user,format('%I.%I',schemaname,tablename),'DELETE')
      OR has_table_privilege(current_user,format('%I.%I',schemaname,tablename),'TRUNCATE')))
);
