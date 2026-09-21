package domain

import (
	"bytes"
	"testing"
)

func TestProfileSnapshotNormalizesApplicabilityAndHashesMembership(t *testing.T) {
	profile := CertificationProfile{
		ProfileRef: " pack/enterprise-activity ", Code: " enterprise_activity ", Name: "Enterprise Activity", Version: "1.0",
		Purpose:   Applicability{Mode: ApplicabilityExplicit, Values: []string{"commercial", "RESEARCH", "commercial"}},
		Actions:   Applicability{Mode: ApplicabilityExplicit, Values: []string{"share"}},
		Consumers: Applicability{Mode: ApplicabilityExplicit, Values: []string{"consumer-b", "consumer-a"}},
		Delivery:  Applicability{Mode: ApplicabilityExplicit, Values: []string{"direct_data"}},
	}
	snapshot, err := profile.Snapshot()
	if err != nil {
		t.Fatalf("snapshot profile: %v", err)
	}
	if snapshot.Purpose.Values[0] != "COMMERCIAL" || snapshot.Actions.Values[0] != "SHARE" || snapshot.Consumers.Values[0] != "consumer-a" || snapshot.Delivery.Values[0] != "DIRECT_DATA" {
		t.Fatalf("profile was not normalized: %#v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("validate snapshot: %v", err)
	}
	tampered := snapshot
	tampered.Actions.Values = append([]string(nil), snapshot.Actions.Values...)
	tampered.Actions.Values[0] = "RAW_EXPORT"
	if err := tampered.Validate(); err == nil {
		t.Fatal("typed profile mutation was accepted without changing the frozen content")
	}
	changed := profile
	changed.Purpose.Values = []string{"commercial", "research", "new-purpose"}
	changedSnapshot, err := changed.Snapshot()
	if err != nil {
		t.Fatalf("snapshot changed profile: %v", err)
	}
	if bytes.Equal(snapshot.Content, changedSnapshot.Content) || snapshot.ContentSHA256 == changedSnapshot.ContentSHA256 {
		t.Fatal("profile membership change did not change frozen content hash")
	}
}

func TestProfileRejectsMissingOrAmbiguousApplicability(t *testing.T) {
	base := CertificationProfile{ProfileRef: "p", Code: "p", Name: "P", Version: "1"}
	cases := []CertificationProfile{
		base,
		func() CertificationProfile {
			p := base
			p.Purpose = Applicability{Mode: ApplicabilityExplicit}
			return p
		}(),
		func() CertificationProfile {
			p := base
			p.Purpose = Applicability{Mode: ApplicabilityAny, Values: []string{"x"}}
			return p
		}(),
	}
	for index, profile := range cases {
		if _, err := profile.Snapshot(); err == nil {
			t.Fatalf("case %d accepted invalid profile", index)
		}
	}
}
