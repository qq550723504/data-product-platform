package infrastructure

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestQuarantineRecordIdentityDistinguishesPayloadButDeduplicatesReplay(t *testing.T) {
	executionID := uuid.New()
	base := QuarantineRecord{
		ExecutionID: executionID,
		InputName:   "energy_raw",
		SourceKey:   "METER-1@2026-09-01T00:00:00Z",
		ReasonCode:  "NEGATIVE_ENERGY_KWH",
	}

	firstPayload, err := json.Marshal(map[string]any{
		"meter_id":     "METER-1",
		"reading_time": "2026-09-01T00:00:00Z",
		"energy_kwh":   "-1",
	})
	if err != nil {
		t.Fatalf("marshal first payload: %v", err)
	}
	replayPayload, err := json.Marshal(map[string]any{
		"energy_kwh":   "-1",
		"reading_time": "2026-09-01T00:00:00Z",
		"meter_id":     "METER-1",
	})
	if err != nil {
		t.Fatalf("marshal replay payload: %v", err)
	}
	distinctPayload, err := json.Marshal(map[string]any{
		"meter_id":     "METER-1",
		"reading_time": "2026-09-01T00:00:00Z",
		"energy_kwh":   "-2",
	})
	if err != nil {
		t.Fatalf("marshal distinct payload: %v", err)
	}

	firstID := quarantineRecordID(base, firstPayload)
	replayID := quarantineRecordID(base, replayPayload)
	distinctID := quarantineRecordID(base, distinctPayload)

	if replayID != firstID {
		t.Fatalf("replay identity changed: first=%s replay=%s", firstID, replayID)
	}
	if distinctID == firstID {
		t.Fatalf("different quarantined payload collapsed to the same identity %s", firstID)
	}
}
