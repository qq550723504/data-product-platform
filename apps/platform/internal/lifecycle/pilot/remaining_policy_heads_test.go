package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCurrentPolicyCheckpointMatchesActualReadOnlyReceipt(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../../docs/acceptance/evidence/issue307-d1/post-v22-policy-heads01/policy-heads.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if string(raw) != string(remainingPolicyCheckpoint) || hex.EncodeToString(sum[:]) != RemainingPolicyCheckpointSHA256 {
		t.Fatal("checkpoint differs from the actual immutable read-only observation")
	}
	var v struct {
		Policies []struct {
			ID       uuid.UUID `json:"id"`
			Version  string    `json:"dataset_version_id"`
			Previous uuid.UUID `json:"supersedes_policy_id"`
		} `json:"policies"`
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Policies) != 5 {
		t.Fatal("five observed successors required")
	}
	heads := RemainingPolicyHeads("business-races")
	if ValidateRemainingPolicyHeads("business-races", heads) != nil {
		t.Fatal("actual current successors rejected")
	}
	for _, row := range v.Policies {
		if heads[row.Version] != row.ID || row.ID == row.Previous {
			t.Fatal("actual successor identity missing")
		}
		changed := RemainingPolicyHeads("business-races")
		changed[row.Version] = row.Previous
		if ValidateRemainingPolicyHeads("business-races", changed) == nil {
			t.Fatal("old predecessor accepted despite current membership cardinality")
		}
	}
	for _, mode := range []string{"missing", "extra", "nil"} {
		changed := RemainingPolicyHeads("business-races")
		for key := range changed {
			if mode == "missing" {
				delete(changed, key)
			} else if mode == "nil" {
				changed[key] = uuid.Nil
			}
			break
		}
		if mode == "extra" {
			changed[uuid.NewString()] = uuid.New()
		}
		if ValidateRemainingPolicyHeads("business-races", changed) == nil {
			t.Fatal(mode, "membership accepted")
		}
	}
	if ValidateRemainingPolicyHeads("late-hold", heads) == nil || ValidateRemainingPolicyHeads("mixed", heads) == nil {
		t.Fatal("business policy leaked into another source")
	}
}
