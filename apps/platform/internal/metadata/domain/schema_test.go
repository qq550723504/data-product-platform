package domain

import (
	"testing"
)

func TestSchemaComparisonPreservesOrderTypeAndConstraint(t *testing.T) {
	baseline := []SchemaColumn{{"meter_id", "TEXT", "NOT_NULL"}, {"energy_kwh", "TEXT", "NOT_NULL"}}
	for _, changed := range [][]SchemaColumn{
		{{"energy_kwh", "TEXT", "NOT_NULL"}, {"meter_id", "TEXT", "NOT_NULL"}},
		{{"meter_id", "TEXT", "NOT_NULL"}, {"energy_kwh", "INT", "NOT_NULL"}},
		{{"meter_id", "TEXT", "NOT_NULL"}, {"energy_kwh", "TEXT", "NULL"}},
		{{"meter_id", "TEXT", "NOT_NULL"}},
	} {
		if Hash(changed) == Hash(baseline) {
			t.Fatal("structural change lost")
		}
	}
	if ValidateColumns([]SchemaColumn{{"energy_kwh", "TEXT", "NOT_NULL"}, {"energy_kwh", "TEXT", "NULL"}}) == nil {
		t.Fatal("duplicate column accepted")
	}
	if ValidateColumns(nil) == nil {
		t.Fatal("unknown schema accepted")
	}
}
