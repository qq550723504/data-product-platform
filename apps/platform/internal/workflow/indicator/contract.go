package indicator

import "time"

// Calculator is the Core-facing port for an industry-pack indicator implementation.
// Concrete industry formulas belong to an industry adapter and are injected at the
// application composition root.
type Calculator interface {
	Calculate(policy Policy, targetPeriod string, input CompanyInput) (CompanyResult, error)
}

type LeaseEvent struct {
	ContractStart time.Time
	ContractEnd   time.Time
	LeaseStatus   string
	DueDate       time.Time
	PaymentDate   *time.Time
}

type EnergyReading struct {
	ReadingTime time.Time
	EnergyKWh   float64
	SourceKey   string
}

type CompanyInput struct {
	EntryDate      *time.Time
	LeaseEvents    []LeaseEvent
	EnergyReadings []EnergyReading
}

type QuarantinedReading struct {
	SourceKey string
	Reason    string
	Reading   EnergyReading
}

type CompanyResult struct {
	TenancyStability   *float64
	RentPerformance    *float64
	EnergyStability    *float64
	ActivityScore      *float64
	ActivityLevel      string
	IndicatorCoverage  float64
	TenancyExplanation map[string]any
	RentExplanation    map[string]any
	EnergyExplanation  map[string]any
	Quarantine         []QuarantinedReading
}
