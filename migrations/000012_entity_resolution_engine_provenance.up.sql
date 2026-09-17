ALTER TABLE entity_match_candidate
    ADD COLUMN match_engine_name varchar(64) NOT NULL DEFAULT 'RULES',
    ADD COLUMN match_engine_version varchar(64) NOT NULL DEFAULT '1',
    ADD COLUMN match_model_version varchar(128) NOT NULL DEFAULT '';

ALTER TABLE entity_mapping
    ADD COLUMN match_engine_name varchar(64) NOT NULL DEFAULT 'RULES',
    ADD COLUMN match_engine_version varchar(64) NOT NULL DEFAULT '1',
    ADD COLUMN match_model_version varchar(128) NOT NULL DEFAULT '';
