package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ApplicabilityMode string

const (
	ApplicabilityAny      ApplicabilityMode = "ANY"
	ApplicabilityExplicit ApplicabilityMode = "EXPLICIT"
)

type Applicability struct {
	Mode   ApplicabilityMode `json:"mode"`
	Values []string          `json:"values,omitempty"`
}

type ScopeRef struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

type ScopeApplicability struct {
	Mode   ApplicabilityMode `json:"mode"`
	Values []ScopeRef        `json:"values,omitempty"`
}

type RightsRequirement struct {
	Required  bool               `json:"required"`
	Purpose   Applicability      `json:"purpose"`
	Actions   Applicability      `json:"actions"`
	Consumers Applicability      `json:"consumers"`
	Scopes    ScopeApplicability `json:"scopes"`
}

type CertificationProfile struct {
	ProfileRef string `json:"profileRef"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Version    string `json:"version"`

	Purpose   Applicability `json:"purpose"`
	Actions   Applicability `json:"actions"`
	Consumers Applicability `json:"consumers"`
	Delivery  Applicability `json:"delivery"`

	RequiredQualityDimensions []string          `json:"requiredQualityDimensions,omitempty"`
	RequiredCriticalRules     []string          `json:"requiredCriticalRules,omitempty"`
	QualityGateRequired       bool              `json:"qualityGateRequired"`
	Rights                    RightsRequirement `json:"rights"`
	ComplianceRequired        bool              `json:"complianceRequired"`
	ContractRequired          bool              `json:"contractRequired"`
	TraceabilityRequired      bool              `json:"traceabilityRequired"`
	EvidenceRequired          bool              `json:"evidenceRequired"`
}

type ProfileSnapshot struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId"`
	CertificationProfile
	ContentSHA256 string     `json:"contentSha256"`
	Content       []byte     `json:"content"`
	CreatedAt     time.Time  `json:"createdAt"`
	CreatedBy     *uuid.UUID `json:"createdBy,omitempty"`
}

var (
	ErrInvalidProfile         = errors.New("invalid certification profile")
	ErrInvalidProfileSnapshot = errors.New("invalid certification profile snapshot")
)

func (p CertificationProfile) Snapshot() (ProfileSnapshot, error) {
	normalized, err := p.normalized()
	if err != nil {
		return ProfileSnapshot{}, err
	}
	content, err := json.Marshal(normalized)
	if err != nil {
		return ProfileSnapshot{}, fmt.Errorf("marshal certification profile snapshot: %w", err)
	}
	digest := sha256.Sum256(content)
	return ProfileSnapshot{
		ID:                   uuid.New(),
		CertificationProfile: normalized,
		ContentSHA256:        hex.EncodeToString(digest[:]),
		Content:              append([]byte(nil), content...),
		CreatedAt:            time.Now().UTC(),
	}, nil
}

func (p CertificationProfile) SnapshotForWorkspace(workspaceID uuid.UUID, actorID *uuid.UUID) (ProfileSnapshot, error) {
	if workspaceID == uuid.Nil {
		return ProfileSnapshot{}, fmt.Errorf("%w: workspace id is required", ErrInvalidProfile)
	}
	snapshot, err := p.Snapshot()
	if err != nil {
		return ProfileSnapshot{}, err
	}
	snapshot.WorkspaceID = workspaceID
	snapshot.CreatedBy = actorID
	return snapshot, nil
}

func (s ProfileSnapshot) Validate() error {
	normalized, err := s.CertificationProfile.normalized()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProfileSnapshot, err)
	}
	if len(s.Content) == 0 || strings.TrimSpace(s.ContentSHA256) == "" {
		return fmt.Errorf("%w: content and hash are required", ErrInvalidProfileSnapshot)
	}
	if s.ID == uuid.Nil {
		return fmt.Errorf("%w: profile snapshot id is required", ErrInvalidProfileSnapshot)
	}
	digest := sha256.Sum256(s.Content)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), s.ContentSHA256) {
		return fmt.Errorf("%w: content hash does not match snapshot", ErrInvalidProfileSnapshot)
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("%w: canonicalize typed snapshot: %v", ErrInvalidProfileSnapshot, err)
	}
	if !bytes.Equal(canonical, s.Content) {
		return fmt.Errorf("%w: typed snapshot does not match frozen content", ErrInvalidProfileSnapshot)
	}
	return nil
}

func (p CertificationProfile) Validate() error {
	_, err := p.normalized()
	return err
}

func (p CertificationProfile) normalized() (CertificationProfile, error) {
	p.ProfileRef = strings.TrimSpace(p.ProfileRef)
	p.Code = strings.TrimSpace(p.Code)
	p.Name = strings.TrimSpace(p.Name)
	p.Version = strings.TrimSpace(p.Version)
	if p.ProfileRef == "" || p.Code == "" || p.Name == "" || p.Version == "" {
		return CertificationProfile{}, fmt.Errorf("%w: profileRef, code, name, and version are required", ErrInvalidProfile)
	}
	var err error
	if p.Purpose, err = normalizeApplicability(p.Purpose, true, "purpose"); err != nil {
		return CertificationProfile{}, err
	}
	if p.Actions, err = normalizeApplicability(p.Actions, true, "actions"); err != nil {
		return CertificationProfile{}, err
	}
	if p.Consumers, err = normalizeApplicability(p.Consumers, false, "consumers"); err != nil {
		return CertificationProfile{}, err
	}
	if p.Delivery, err = normalizeApplicability(p.Delivery, true, "delivery"); err != nil {
		return CertificationProfile{}, err
	}
	p.RequiredQualityDimensions = normalizeCodes(p.RequiredQualityDimensions)
	p.RequiredCriticalRules = normalizeCodes(p.RequiredCriticalRules)
	if p.Rights.Required {
		if p.Rights.Purpose, err = normalizeApplicability(p.Rights.Purpose, true, "rights.purpose"); err != nil {
			return CertificationProfile{}, err
		}
		if p.Rights.Actions, err = normalizeApplicability(p.Rights.Actions, true, "rights.actions"); err != nil {
			return CertificationProfile{}, err
		}
		if p.Rights.Consumers, err = normalizeApplicability(p.Rights.Consumers, false, "rights.consumers"); err != nil {
			return CertificationProfile{}, err
		}
		if p.Rights.Scopes, err = normalizeScopes(p.Rights.Scopes, "rights.scopes"); err != nil {
			return CertificationProfile{}, err
		}
	}
	return p, nil
}

func normalizeApplicability(a Applicability, uppercase bool, name string) (Applicability, error) {
	a.Mode = ApplicabilityMode(strings.ToUpper(strings.TrimSpace(string(a.Mode))))
	if a.Mode != ApplicabilityAny && a.Mode != ApplicabilityExplicit {
		return Applicability{}, fmt.Errorf("%w: %s mode must be ANY or EXPLICIT", ErrInvalidProfile, name)
	}
	values := make([]string, 0, len(a.Values))
	seen := make(map[string]struct{}, len(a.Values))
	for _, raw := range a.Values {
		value := strings.TrimSpace(raw)
		if uppercase {
			value = strings.ToUpper(value)
		}
		if value == "" {
			return Applicability{}, fmt.Errorf("%w: %s contains an empty member", ErrInvalidProfile, name)
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	if a.Mode == ApplicabilityAny && len(values) != 0 {
		return Applicability{}, fmt.Errorf("%w: %s ANY cannot contain explicit members", ErrInvalidProfile, name)
	}
	if a.Mode == ApplicabilityExplicit && len(values) == 0 {
		return Applicability{}, fmt.Errorf("%w: %s EXPLICIT requires at least one member", ErrInvalidProfile, name)
	}
	sort.Strings(values)
	a.Values = values
	return a, nil
}

func normalizeScopes(a ScopeApplicability, name string) (ScopeApplicability, error) {
	a.Mode = ApplicabilityMode(strings.ToUpper(strings.TrimSpace(string(a.Mode))))
	if a.Mode != ApplicabilityAny && a.Mode != ApplicabilityExplicit {
		return ScopeApplicability{}, fmt.Errorf("%w: %s mode must be ANY or EXPLICIT", ErrInvalidProfile, name)
	}
	values := make([]ScopeRef, 0, len(a.Values))
	seen := map[string]struct{}{}
	for _, scope := range a.Values {
		scope.Type = strings.ToUpper(strings.TrimSpace(scope.Type))
		scope.Ref = strings.TrimSpace(scope.Ref)
		if scope.Type == "" || scope.Ref == "" {
			return ScopeApplicability{}, fmt.Errorf("%w: %s contains an incomplete member", ErrInvalidProfile, name)
		}
		key := scope.Type + "\x00" + scope.Ref
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			values = append(values, scope)
		}
	}
	if a.Mode == ApplicabilityAny && len(values) != 0 {
		return ScopeApplicability{}, fmt.Errorf("%w: %s ANY cannot contain explicit members", ErrInvalidProfile, name)
	}
	if a.Mode == ApplicabilityExplicit && len(values) == 0 {
		return ScopeApplicability{}, fmt.Errorf("%w: %s EXPLICIT requires at least one member", ErrInvalidProfile, name)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Type == values[j].Type {
			return values[i].Ref < values[j].Ref
		}
		return values[i].Type < values[j].Type
	})
	a.Values = values
	return a, nil
}

func normalizeCodes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.ToUpper(strings.TrimSpace(raw))
		if value != "" {
			if _, exists := seen[value]; !exists {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}

func (a Applicability) Covers(value string, uppercase bool) bool {
	value = strings.TrimSpace(value)
	if uppercase {
		value = strings.ToUpper(value)
	}
	if a.Mode == ApplicabilityAny {
		return true
	}
	for _, member := range a.Values {
		if member == value {
			return true
		}
	}
	return false
}

func (a Applicability) CoversApplicability(required Applicability, uppercase bool) bool {
	if required.Mode == ApplicabilityAny {
		return a.Mode == ApplicabilityAny
	}
	if a.Mode == ApplicabilityAny {
		return true
	}
	for _, value := range required.Values {
		if !a.Covers(value, uppercase) {
			return false
		}
	}
	return true
}

func (a ScopeApplicability) Covers(required ScopeApplicability) bool {
	if required.Mode == ApplicabilityAny {
		return a.Mode == ApplicabilityAny
	}
	if a.Mode == ApplicabilityAny {
		return true
	}
	for _, requiredScope := range required.Values {
		found := false
		for _, scope := range a.Values {
			if scope == requiredScope {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
