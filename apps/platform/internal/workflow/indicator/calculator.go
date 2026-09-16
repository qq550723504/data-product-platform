package indicator

import (
	"fmt"
	"math"
	"sort"
	"time"
)

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
	TenancyStability *float64
	RentPerformance  *float64
	EnergyStability  *float64
	ActivityScore    *float64
	ActivityLevel    string
	IndicatorCoverage float64
	TenancyExplanation map[string]any
	RentExplanation    map[string]any
	EnergyExplanation  map[string]any
	Quarantine         []QuarantinedReading
}

func Calculate(policy Policy, targetPeriod string, input CompanyInput) (CompanyResult, error) {
	targetStart, targetEnd, err := targetPeriodBounds(targetPeriod)
	if err != nil {
		return CompanyResult{}, err
	}
	tenancyDef, _ := policy.Definition("tenancy_stability")
	rentDef, _ := policy.Definition("rent_performance")
	energyDef, _ := policy.Definition("energy_stability")
	activityDef, _ := policy.Definition("activity_score")

	tenancy, tenancyExplanation := calculateTenancy(tenancyDef.Parameters, targetStart, targetEnd, input.EntryDate, input.LeaseEvents)
	rent, rentExplanation := calculateRent(rentDef.Parameters, targetEnd, input.LeaseEvents)
	energy, energyExplanation, quarantine := calculateEnergy(energyDef.Parameters, targetStart, input.EnergyReadings)

	result := CompanyResult{
		TenancyStability:   roundOptional(tenancy, policy.Spec.Rounding.Decimals),
		RentPerformance:    roundOptional(rent, policy.Spec.Rounding.Decimals),
		EnergyStability:    roundOptional(energy, policy.Spec.Rounding.Decimals),
		TenancyExplanation: tenancyExplanation,
		RentExplanation:    rentExplanation,
		EnergyExplanation:  energyExplanation,
		Quarantine:         quarantine,
	}

	available := 0
	for _, value := range []*float64{result.TenancyStability, result.RentPerformance, result.EnergyStability} {
		if value != nil {
			available++
		}
	}
	result.IndicatorCoverage = round(float64(available)/3*100, policy.Spec.Rounding.Decimals)

	if available != 3 {
		result.ActivityLevel = "INSUFFICIENT_DATA"
		return result, nil
	}

	weights := activityDef.Parameters.Weights
	score := *result.TenancyStability*weights["tenancy_stability"] +
		*result.RentPerformance*weights["rent_performance"] +
		*result.EnergyStability*weights["energy_stability"]
	score = round(score, policy.Spec.Rounding.Decimals)
	result.ActivityScore = &score
	result.ActivityLevel = classifyLevel(activityDef.Parameters, score)
	return result, nil
}

func calculateTenancy(params IndicatorParameters, targetStart, targetEnd time.Time, entryDate *time.Time, leases []LeaseEvent) (*float64, map[string]any) {
	explanation := map[string]any{}
	if entryDate == nil || len(leases) == 0 {
		return nil, explanation
	}
	months := fullCalendarMonths(*entryDate, targetEnd)
	if months < 0 {
		months = 0
	}
	if months > params.TenureFullScoreMonths {
		months = params.TenureFullScoreMonths
	}
	tenureScore := 0.0
	if params.TenureFullScoreMonths > 0 {
		tenureScore = math.Min(100, float64(months)/float64(params.TenureFullScoreMonths)*100)
	}
	active := false
	for _, lease := range leases {
		if lease.LeaseStatus == "ACTIVE" && !lease.ContractStart.After(targetEnd) && !lease.ContractEnd.Before(targetStart) {
			active = true
			break
		}
	}
	leaseScore := 0.0
	if active {
		leaseScore = 100
	}
	explanation["tenure_months"] = months
	explanation["tenure_score"] = tenureScore
	explanation["current_lease_active"] = active
	explanation["current_lease_score"] = leaseScore
	value := params.TenureWeight*tenureScore + params.CurrentLeaseWeight*leaseScore
	return &value, explanation
}

func calculateRent(params IndicatorParameters, targetEnd time.Time, leases []LeaseEvent) (*float64, map[string]any) {
	explanation := map[string]any{
		"due_event_count":       0,
		"on_time_count":        0,
		"late_1_to_7_count":    0,
		"late_8_to_30_count":   0,
		"late_over_30_count":   0,
		"overdue_unpaid_count": 0,
	}
	if len(leases) == 0 {
		return nil, explanation
	}
	lookback := params.LookbackMonths
	if lookback <= 0 {
		lookback = 12
	}
	windowStart := time.Date(targetEnd.Year(), targetEnd.Month(), 1, 0, 0, 0, 0, targetEnd.Location()).AddDate(0, -(lookback - 1), 0)
	total := 0.0
	count := 0
	for _, event := range leases {
		if event.DueDate.IsZero() || event.DueDate.Before(windowStart) || event.DueDate.After(targetEnd) {
			continue
		}
		class, score := classifyRentEvent(event.DueDate, event.PaymentDate, targetEnd, params.EventScores)
		if class == "" {
			continue
		}
		count++
		total += score
		explanation["due_event_count"] = count
		switch class {
		case "ON_TIME":
			explanation["on_time_count"] = explanation["on_time_count"].(int) + 1
		case "LATE_1_TO_7_DAYS":
			explanation["late_1_to_7_count"] = explanation["late_1_to_7_count"].(int) + 1
		case "LATE_8_TO_30_DAYS":
			explanation["late_8_to_30_count"] = explanation["late_8_to_30_count"].(int) + 1
		case "LATE_OVER_30_DAYS":
			explanation["late_over_30_count"] = explanation["late_over_30_count"].(int) + 1
		case "OVERDUE_UNPAID":
			explanation["overdue_unpaid_count"] = explanation["overdue_unpaid_count"].(int) + 1
		}
	}
	if count < params.MinimumDueEvents {
		return nil, explanation
	}
	value := total / float64(count)
	return &value, explanation
}

func classifyRentEvent(due time.Time, paid *time.Time, asOf time.Time, scores map[string]float64) (string, float64) {
	if paid == nil {
		if !due.After(asOf) {
			return "OVERDUE_UNPAID", scores["OVERDUE_UNPAID"]
		}
		return "", 0
	}
	if !paid.After(due) {
		return "ON_TIME", scores["ON_TIME"]
	}
	lateDays := int(paid.Sub(due).Hours() / 24)
	switch {
	case lateDays <= 7:
		return "LATE_1_TO_7_DAYS", scores["LATE_1_TO_7_DAYS"]
	case lateDays <= 30:
		return "LATE_8_TO_30_DAYS", scores["LATE_8_TO_30_DAYS"]
	default:
		return "LATE_OVER_30_DAYS", scores["LATE_OVER_30_DAYS"]
	}
}

func calculateEnergy(params IndicatorParameters, targetStart time.Time, readings []EnergyReading) (*float64, map[string]any, []QuarantinedReading) {
	explanation := map[string]any{}
	lookback := params.LookbackMonths
	if lookback <= 0 {
		lookback = 6
	}
	windowStart := targetStart.AddDate(0, -(lookback - 1), 0)
	windowEnd := targetStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
	monthly := map[string]float64{}
	observed := map[string]struct{}{}
	quarantine := make([]QuarantinedReading, 0)
	for _, reading := range readings {
		if reading.ReadingTime.Before(windowStart) || reading.ReadingTime.After(windowEnd) {
			continue
		}
		key := reading.ReadingTime.Format("2006-01")
		observed[key] = struct{}{}
		if reading.EnergyKWh < 0 {
			quarantine = append(quarantine, QuarantinedReading{SourceKey: reading.SourceKey, Reason: "NEGATIVE_ENERGY_KWH", Reading: reading})
			continue
		}
		monthly[key] += reading.EnergyKWh
	}

	keys := make([]string, 0, len(observed))
	for key := range observed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		explanation["quarantined_record_count"] = len(quarantine)
		return nil, explanation, quarantine
	}
	first, _ := time.Parse("2006-01", keys[0])
	targetMonth := time.Date(targetStart.Year(), targetStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	expectedMonths := monthDistance(first, targetMonth) + 1
	if expectedMonths > lookback {
		expectedMonths = lookback
	}
	if expectedMonths < 1 {
		expectedMonths = 1
	}

	values := make([]float64, 0, len(monthly))
	for _, value := range monthly {
		values = append(values, value)
	}
	validMonths := len(values)
	coverageScore := float64(validMonths) / float64(expectedMonths) * 100
	explanation["expected_months"] = expectedMonths
	explanation["valid_months"] = validMonths
	explanation["coverage_score"] = coverageScore
	explanation["quarantined_record_count"] = len(quarantine)
	if validMonths < params.MinimumValidMonths {
		return nil, explanation, quarantine
	}
	mean := arithmeticMean(values)
	if mean <= 0 {
		return nil, explanation, quarantine
	}
	stddev := populationStddev(values, mean)
	cv := stddev / mean
	variability := clamp(100*(1-cv/params.MaximumReferenceCV), 0, 100)
	explanation["monthly_mean_kwh"] = mean
	explanation["monthly_stddev_kwh"] = stddev
	explanation["coefficient_of_variation"] = cv
	explanation["variability_score"] = variability
	value := params.VariabilityWeight*variability + params.CoverageWeight*coverageScore
	return &value, explanation, quarantine
}

func classifyLevel(params IndicatorParameters, score float64) string {
	if level, ok := params.Levels["HIGH"]; ok && score >= level.MinimumInclusive {
		return "HIGH"
	}
	if level, ok := params.Levels["MEDIUM"]; ok && score >= level.MinimumInclusive {
		if level.MaximumExclusive == nil || score < *level.MaximumExclusive {
			return "MEDIUM"
		}
	}
	return "LOW"
}

func targetPeriodBounds(value string) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse target period: %w", err)
	}
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	return start, end, nil
}

func fullCalendarMonths(start, end time.Time) int {
	months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
	if end.Day() < start.Day() {
		months--
	}
	return months
}

func monthDistance(start, end time.Time) int {
	return (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
}

func arithmeticMean(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func populationStddev(values []float64, mean float64) float64 {
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}

func clamp(value, minimum, maximum float64) float64 {
	return math.Min(maximum, math.Max(minimum, value))
}

func roundOptional(value *float64, decimals int) *float64 {
	if value == nil {
		return nil
	}
	rounded := round(*value, decimals)
	return &rounded
}

func round(value float64, decimals int) float64 {
	factor := math.Pow10(decimals)
	return math.Floor(value*factor+0.5) / factor
}
