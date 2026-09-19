-- Freeze the handler obligation on the event itself.
--
-- Before this migration the required handler set was resolved from the
-- dispatcher's in-memory routing table at dispatch time. That made an
-- obligation mutable: an event first claimed under one deployment profile
-- (for example governance projection enabled) could be re-interpreted as
-- "retention only" by a later process started with a different profile, and
-- published without ever satisfying the original obligation.
--
-- The obligation is now persisted with the event:
--   routing_version   the routing version that decided the obligation
--   required_handlers the handler names the event must be confirmed by
--
-- Both stay NULL for events recorded before the routing table was known; the
-- first dispatcher to claim such an event freezes them inside the claim
-- transaction, so concurrent instances cannot disagree. retention-only events
-- are stored as routing_version <> '' with an empty required_handlers array,
-- which is deliberately distinguishable from "not yet frozen" (NULL).
--
-- Historical rows are not backfilled. Backfilling would invent an obligation
-- that was never declared when the event was written; leaving them NULL lets
-- the first claimer freeze them under a routing version that is explicit in
-- the routing table.

ALTER TABLE outbox_event
    ADD COLUMN routing_version varchar(64),
    ADD COLUMN required_handlers text[];

ALTER TABLE outbox_event
    ADD CONSTRAINT ck_outbox_required_handlers_consistency
    CHECK (
        (routing_version IS NULL AND required_handlers IS NULL)
        OR (routing_version IS NOT NULL AND required_handlers IS NOT NULL)
    );

COMMENT ON COLUMN outbox_event.routing_version IS
    'Routing version that froze required_handlers; NULL means the obligation is not frozen yet.';

COMMENT ON COLUMN outbox_event.required_handlers IS
    'Handler names that must confirm this event before it is PUBLISHED. Empty array is an explicit retention-only obligation.';
