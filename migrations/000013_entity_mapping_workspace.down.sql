-- This migration is forward-only.
--
-- Rolling it back would drop the immutable entity_mapping_decision history and
-- would have to shrink the workspace-scoped uniqueness back to a global key,
-- which can silently discard rows that are only distinguishable by workspace.
-- Per AGENTS.md §3 and #99, historical facts are never overwritten or deleted,
-- so there is no lossless rollback. Restore from a backup that predates the
-- migration if a rollback is genuinely required.
DO $$
BEGIN
    RAISE EXCEPTION '000013_entity_mapping_workspace is forward-only; restore from a pre-migration backup instead of rolling back';
END $$;
