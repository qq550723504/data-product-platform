DO $$
BEGIN
    LOCK TABLE certification_profile,
        certification_profile_purpose,
        certification_profile_action,
        certification_profile_consumer,
        certification_profile_delivery,
        certification_profile_quality_dimension,
        certification_profile_critical_rule,
        certification_profile_rights_purpose,
        certification_profile_rights_action,
        certification_profile_rights_consumer,
        certification_profile_rights_scope
        IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM certification_profile) THEN
        RAISE EXCEPTION 'refusing to downgrade certification_profile with historical rows';
    END IF;
END;
$$;

DROP TABLE certification_profile_rights_scope;
DROP TABLE certification_profile_rights_consumer;
DROP TABLE certification_profile_rights_action;
DROP TABLE certification_profile_rights_purpose;
DROP TABLE certification_profile_critical_rule;
DROP TABLE certification_profile_quality_dimension;
DROP TABLE certification_profile_delivery;
DROP TABLE certification_profile_consumer;
DROP TABLE certification_profile_action;
DROP TABLE certification_profile_purpose;
DROP TABLE certification_profile;

DROP FUNCTION validate_certification_profile_membership();
DROP FUNCTION prevent_certification_profile_membership_mutation();
DROP FUNCTION prevent_certification_profile_mutation();
