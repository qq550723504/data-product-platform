package indicator

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	workflowindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/indicator"
)

func TestReferenceVectorsV1(t *testing.T) {
	policy := loadReferencePolicy(t)

	tests := []struct {
		name       string
		input      workflowindicator.CompanyInput
		tenancy    *float64
		rent       *float64
		energy     *float64
		activity   *float64
		level      string
		coverage   float64
		quarantine int
	}{
		{
			name: "STAR-CLOUD",
			input: workflowindicator.CompanyInput{
				EntryDate: ptrDate(t, "2022-03-01"),
				LeaseEvents: []workflowindicator.LeaseEvent{
					lease(t, "2025-01-01", "2025-12-31", "2025-01-05", "2025-01-04"),
					lease(t, "2025-01-01", "2025-12-31", "2025-02-05", "2025-02-07"),
					lease(t, "2025-01-01", "2025-12-31", "2025-03-05", "2025-03-05"),
				},
				EnergyReadings: []workflowindicator.EnergyReading{
					energy(t, "M-001-01", "2025-01-31T23:59:00+08:00", 12850),
					energy(t, "M-001-02", "2025-02-28T23:59:00+08:00", 13120),
					energy(t, "M-001-03", "2025-03-31T23:59:00+08:00", 12990),
				},
			},
			tenancy: number(100), rent: number(90), energy: number(98.02), activity: number(96.01), level: "HIGH", coverage: 100,
		},
		{
			name: "QINGHE",
			input: workflowindicator.CompanyInput{
				EntryDate: ptrDate(t, "2023-07-15"),
				LeaseEvents: []workflowindicator.LeaseEvent{
					lease(t, "2025-01-01", "2025-12-31", "2025-01-05", "2025-01-05"),
					lease(t, "2025-01-01", "2025-12-31", "2025-02-05", "2025-02-05"),
					lease(t, "2025-01-01", "2025-12-31", "2025-03-05", "2025-03-18"),
				},
				EnergyReadings: []workflowindicator.EnergyReading{
					energy(t, "M-002-01", "2025-01-31T23:59:00+08:00", 8240),
					energy(t, "M-002-02", "2025-02-28T23:59:00+08:00", 8160),
					energy(t, "M-002-03", "2025-03-31T23:59:00+08:00", 7990),
				},
			},
			tenancy: number(68.89), rent: number(80), energy: number(97.01), activity: number(81.97), level: "HIGH", coverage: 100,
		},
		{
			name: "LANTU",
			input: workflowindicator.CompanyInput{
				EntryDate: ptrDate(t, "2021-01-10"),
				LeaseEvents: []workflowindicator.LeaseEvent{
					lease(t, "2025-01-01", "2025-12-31", "2025-01-05", "2025-01-03"),
					lease(t, "2025-01-01", "2025-12-31", "2025-02-05", "2025-02-03"),
				},
				EnergyReadings: []workflowindicator.EnergyReading{
					energy(t, "M-003-01", "2025-01-31T23:59:00+08:00", 22400),
					energy(t, "M-003-02", "2025-02-28T23:59:00+08:00", 21800),
					energy(t, "M-003-03", "2025-03-31T23:59:00+08:00", 22150),
				},
			},
			tenancy: number(100), rent: number(100), energy: number(97.40), activity: number(99.13), level: "HIGH", coverage: 100,
		},
		{
			name: "YUNFAN-INSUFFICIENT",
			input: workflowindicator.CompanyInput{
				EntryDate:   ptrDate(t, "2024-02-20"),
				LeaseEvents: []workflowindicator.LeaseEvent{lease(t, "2025-02-20", "2026-02-19", "2025-03-05", "2025-03-05")},
				EnergyReadings: []workflowindicator.EnergyReading{
					energy(t, "M-004-02", "2025-02-28T23:59:00+08:00", 6100),
					energy(t, "M-004-03", "2025-03-31T23:59:00+08:00", 6250),
				},
			},
			tenancy: number(55.28), rent: nil, energy: nil, activity: nil, level: "INSUFFICIENT_DATA", coverage: 33.33,
		},
		{
			name: "HAIYUE-QUARANTINE",
			input: workflowindicator.CompanyInput{
				EntryDate:   ptrDate(t, "2020-06-08"),
				LeaseEvents: []workflowindicator.LeaseEvent{leaseUnpaid(t, "2025-01-01", "2025-12-31", "2025-01-05")},
				EnergyReadings: []workflowindicator.EnergyReading{
					energy(t, "M-005-01", "2025-01-31T23:59:00+08:00", 15400),
					energy(t, "M-005-02", "2025-02-28T23:59:00+08:00", -999),
					energy(t, "M-005-03", "2025-03-31T23:59:00+08:00", 14980),
				},
			},
			tenancy: number(100), rent: nil, energy: nil, activity: nil, level: "INSUFFICIENT_DATA", coverage: 33.33, quarantine: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewCalculator().Calculate(policy, "2025-03", tt.input)
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			assertOptional(t, "tenancy", result.TenancyStability, tt.tenancy)
			assertOptional(t, "rent", result.RentPerformance, tt.rent)
			assertOptional(t, "energy", result.EnergyStability, tt.energy)
			assertOptional(t, "activity", result.ActivityScore, tt.activity)
			if result.ActivityLevel != tt.level {
				t.Fatalf("activity level = %s, want %s", result.ActivityLevel, tt.level)
			}
			if result.IndicatorCoverage != tt.coverage {
				t.Fatalf("coverage = %.2f, want %.2f", result.IndicatorCoverage, tt.coverage)
			}
			if len(result.Quarantine) != tt.quarantine {
				t.Fatalf("quarantine count = %d, want %d", len(result.Quarantine), tt.quarantine)
			}
		})
	}
}

func TestRentBoundaryClassification(t *testing.T) {
	policy := loadReferencePolicy(t)
	definition, _ := policy.Definition("rent_performance")
	scores := definition.Parameters.EventScores
	due := mustDate(t, "2025-03-05")
	asOf := mustDate(t, "2025-03-31")

	cases := []struct {
		paid  *time.Time
		class string
		score float64
	}{
		{ptrDate(t, "2025-03-05"), "ON_TIME", 100},
		{ptrDate(t, "2025-03-06"), "LATE_1_TO_7_DAYS", 70},
		{ptrDate(t, "2025-03-12"), "LATE_1_TO_7_DAYS", 70},
		{ptrDate(t, "2025-03-13"), "LATE_8_TO_30_DAYS", 40},
		{ptrDate(t, "2025-04-05"), "LATE_OVER_30_DAYS", 20},
		{nil, "OVERDUE_UNPAID", 0},
	}
	for _, tt := range cases {
		class, score := classifyRentEvent(due, tt.paid, asOf, scores)
		if class != tt.class || score != tt.score {
			t.Fatalf("classify got (%s, %.0f), want (%s, %.0f)", class, score, tt.class, tt.score)
		}
	}
}

func TestEnergyAggregationUsesPackCalendarTimezone(t *testing.T) {
	policy := loadReferencePolicy(t)
	if policy.Spec.CalendarTimezone != "Asia/Shanghai" {
		t.Fatalf("calendar timezone = %q, want Asia/Shanghai", policy.Spec.CalendarTimezone)
	}
	result, err := NewCalculator().Calculate(policy, "2025-03", workflowindicator.CompanyInput{
		EnergyReadings: []workflowindicator.EnergyReading{
			energy(t, "TZ-JAN", "2024-12-31T16:30:00Z", 100),
			energy(t, "TZ-FEB", "2025-01-31T16:30:00Z", 100),
			energy(t, "TZ-MAR", "2025-02-28T16:30:00Z", 100),
		},
	})
	if err != nil {
		t.Fatalf("calculate timezone boundary: %v", err)
	}
	assertOptional(t, "energy", result.EnergyStability, number(100))
	if got := result.EnergyExplanation["expected_months"]; got != 3 {
		t.Fatalf("expected_months = %v, want 3", got)
	}
	if got := result.EnergyExplanation["valid_months"]; got != 3 {
		t.Fatalf("valid_months = %v, want 3", got)
	}
}

func loadReferencePolicy(t *testing.T) workflowindicator.Policy {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", ".."))
	policy, err := workflowindicator.LoadPolicy(filepath.Join(root, "industry-packs", "park", "indicators", "enterprise-activity-v1.yaml"))
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	return policy
}

func lease(t *testing.T, start, end, due, paid string) workflowindicator.LeaseEvent {
	t.Helper()
	return workflowindicator.LeaseEvent{
		ContractStart: mustDate(t, start),
		ContractEnd:   mustDate(t, end),
		LeaseStatus:   "ACTIVE",
		DueDate:       mustDate(t, due),
		PaymentDate:   ptrDate(t, paid),
	}
}

func leaseUnpaid(t *testing.T, start, end, due string) workflowindicator.LeaseEvent {
	t.Helper()
	return workflowindicator.LeaseEvent{
		ContractStart: mustDate(t, start),
		ContractEnd:   mustDate(t, end),
		LeaseStatus:   "ACTIVE",
		DueDate:       mustDate(t, due),
	}
}

func energy(t *testing.T, key, timestamp string, kwh float64) workflowindicator.EnergyReading {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Fatalf("parse timestamp %s: %v", timestamp, err)
	}
	return workflowindicator.EnergyReading{ReadingTime: parsed, EnergyKWh: kwh, SourceKey: key}
}

func ptrDate(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed := mustDate(t, value)
	return &parsed
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("parse date %s: %v", value, err)
	}
	return parsed
}

func number(value float64) *float64 { return &value }

func assertOptional(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s = %.2f, want %.2f", name, *got, *want)
	}
}
