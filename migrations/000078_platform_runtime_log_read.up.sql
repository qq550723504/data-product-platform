-- Separate runtime-log read authority. No grants are added or broadened.
ALTER TABLE platform_workspace_grant DROP CONSTRAINT platform_workspace_grant_capabilities_check;
ALTER TABLE platform_workspace_grant ADD CONSTRAINT platform_workspace_grant_capabilities_check CHECK
 (capabilities <@ ARRAY['HUMAN_DECISION','IDENTITY_BINDING_ADMIN','BUSINESS_ACCEPTANCE_READ','BUSINESS_ACCEPTANCE_DRAFT','BUSINESS_ACCEPTANCE_BASIS_CONFIRM','BUSINESS_RECEIPT_RECORD','PLATFORM_MANAGEMENT_READ','PLATFORM_RUNTIME_LOG_READ']::text[]);
