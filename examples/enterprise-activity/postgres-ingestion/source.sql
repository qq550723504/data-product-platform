-- Isolated synthetic source only. No Core schema or migration is modified.
REVOKE ALL ON DATABASE dpp_ingestion_source FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE ROLE fixture_owner NOLOGIN;
CREATE SCHEMA dpp_source AUTHORIZATION fixture_owner;
SET ROLE fixture_owner;
CREATE TABLE dpp_source.enterprise (
  source_company_id text NOT NULL, company_name text NOT NULL,
  unified_social_credit_code text NOT NULL, legal_representative text NOT NULL,
  registered_address text NOT NULL, entry_date text NOT NULL,
  company_status text NOT NULL, contact_name text NOT NULL,
  mobile text NOT NULL, email text NOT NULL
);
CREATE TABLE dpp_source.lease (
  lease_id text NOT NULL, source_company_id text NOT NULL, company_name text NOT NULL,
  contract_start text NOT NULL, contract_end text NOT NULL, rent_amount text NOT NULL,
  payment_due_date text NOT NULL, payment_date text NOT NULL, lease_status text NOT NULL
);
CREATE TABLE dpp_source.energy (
  meter_id text NOT NULL, source_company_id text NOT NULL, company_name text NOT NULL,
  reading_time text NOT NULL, energy_kwh text NOT NULL
);
RESET ROLE;
-- dpp_reader is provisioned with an ephemeral secret over stdin by smoke.py.
GRANT CONNECT ON DATABASE dpp_ingestion_source TO dpp_reader;
GRANT USAGE ON SCHEMA dpp_source TO dpp_reader;
GRANT SELECT ON dpp_source.enterprise, dpp_source.lease, dpp_source.energy TO dpp_reader;
ALTER ROLE dpp_reader SET default_transaction_read_only = on;
ALTER ROLE dpp_reader SET search_path = pg_catalog, dpp_source;
ALTER ROLE dpp_reader SET statement_timeout = '15s';
