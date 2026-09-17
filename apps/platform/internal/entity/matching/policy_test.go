package matching

import (
	"math"
	"testing"
)

func TestValidateThresholds(t *testing.T) {
	nan := math.NaN()
	tests := []struct {
		name      string
		threshold Thresholds
		wantErr   bool
	}{
		{name: "valid", threshold: Thresholds{ReviewMinimum: 0.75, AutoMatchMinimum: 0.95}},
		{name: "missing", threshold: Thresholds{}, wantErr: true},
		{name: "review above auto", threshold: Thresholds{ReviewMinimum: 0.96, AutoMatchMinimum: 0.95}, wantErr: true},
		{name: "auto over one", threshold: Thresholds{ReviewMinimum: 0.75, AutoMatchMinimum: 1.01}, wantErr: true},
		{name: "review over one", threshold: Thresholds{ReviewMinimum: 1.01, AutoMatchMinimum: 1}, wantErr: true},
		{name: "nan review", threshold: Thresholds{ReviewMinimum: nan, AutoMatchMinimum: 0.95}, wantErr: true},
		{name: "nan auto", threshold: Thresholds{ReviewMinimum: 0.75, AutoMatchMinimum: nan}, wantErr: true},
		{name: "infinite auto", threshold: Thresholds{ReviewMinimum: 0.75, AutoMatchMinimum: math.Inf(1)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateThresholds(tt.threshold)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateThresholds(%#v) error = %v, wantErr=%v", tt.threshold, err, tt.wantErr)
			}
		})
	}
}
