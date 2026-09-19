ALTER TABLE outbox_event
    DROP CONSTRAINT ck_outbox_required_handlers_consistency;

ALTER TABLE outbox_event
    DROP COLUMN required_handlers,
    DROP COLUMN routing_version;
