package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
)

// This fresh permission never authorizes a DELETE, producer, reader or old
// execution resume. The trusted host separately binds the actual bootstrap CID.
type p1ReadOnlyPermission struct {
	Enabled                                                           bool
	AuditID                                                           uuid.UUID
	Group, Purpose, SourceCandidateSHA256, BinarySHA256, Core, Native string
	CopyID, AttemptID                                                 uuid.UUID
	NotBefore, ExpiresAt                                              time.Time
	MaxProviderDeletes                                                int
}

func validateReadOnlyPermission(p p1ReadOnlyPermission, source, binary string, at time.Time) error {
	if !p.Enabled || p.AuditID == uuid.Nil || p.AuditID.String() == execution || p.Group != "business-races" ||
		p.Purpose != "POST_V22_P1_READONLY_RECONCILIATION" || p.MaxProviderDeletes != 0 ||
		p.SourceCandidateSHA256 != "f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243" || p.SourceCandidateSHA256 != source ||
		len(p.BinarySHA256) != 64 || p.BinarySHA256 != binary ||
		p.Core != "9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0" ||
		p.Native != "f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce" ||
		p.CopyID.String() != "5a4c9789-3e67-4e54-ab58-128d111cbd65" || p.AttemptID.String() != "009f4317-8855-4f99-80ec-cafc225f654e" ||
		p.NotBefore.IsZero() || !p.ExpiresAt.After(p.NotBefore) || p.ExpiresAt.Sub(p.NotBefore) > 40*time.Minute || at.Before(p.NotBefore) || !at.Before(p.ExpiresAt) {
		return errors.New("new P1 read-only identity permission denied")
	}
	return nil
}

func authorizedReadOnly() error {
	raw, err := os.ReadFile("/run/private/p1-readonly-permission.json")
	if err != nil || len(raw) > 8192 {
		return errors.New("new P1 read-only permission absent")
	}
	var p p1ReadOnlyPermission
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("new P1 read-only permission invalid")
	}
	source, err := os.ReadFile("/run/private/candidate.json")
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	return validateReadOnlyPermission(p, digest(source), digest(binary), time.Now().UTC())
}
