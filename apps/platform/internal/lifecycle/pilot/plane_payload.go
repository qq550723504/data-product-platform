package pilot

import "strings"

const PlanePeriod = "2025-07"
const PlanePurpose = "LIFECYCLE_SYNTHETIC_FIXTURE"
const PlaneConsumer = "SYNTHETIC_LIFECYCLE_307_CONSUMER"

// Two explicit synthetic tables. The combined lease/energy fixture is bound to
// both named inputs; the existing native engine still owns business processing.
// A large inert note exercises real TCP backpressure without extra source objects.
func PlaneSyntheticPayloads() [2][]byte {
	a := []byte("source_company_id,company_name,unified_social_credit_code,legal_representative,registered_address,entry_date,company_status\nSYN-001,SyntheticLifecycle307Company,SYNTHETIC307CODE000,SyntheticPerson,SyntheticAddress,2022-01-01,ACTIVE\n")
	b := []byte("lease_id,meter_id,source_company_id,company_name,contract_start,contract_end,rent_amount,payment_due_date,payment_date,lease_status,reading_time,energy_kwh,notes\nSYN-LEASE,SYN-METER,SYN-001,SyntheticLifecycle307Company,2025-01-01,2025-12-31,100,2025-07-05,2025-07-04,ACTIVE,2025-07-31T23:59:00+08:00,10," + strings.Repeat("S", 126<<10) + "\n")
	return [2][]byte{a, b}
}
