-- Freeze the handler obligation on every outbox event at append time.
--
-- routing_version identifies the routing contract that decided the obligation.
-- required_handlers lists the handlers that must confirm before the event may be
-- marked PUBLISHED. An empty array is an explicit retention-only obligation.
--
-- This is a pre-production schema: there are no legacy events whose obligation
-- needs to be inferred later by a dispatcher.

ALTER TABLE outbox_event
    ADD COLUMN routing_version varchar(64) NOT NULL,
    ADD COLUMN required_handlers text[] NOT NULL;

COMMENT ON COLUMN outbox_event.routing_version IS
    'Routing version that froze required_handlers when the event was appended.';

COMMENT ON COLUMN outbox_event.required_handlers IS
    'Handler names that must confirm this event before it is PUBLISHED. Empty array is an explicit retention-only obligation.';
