package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type VersionStatus string

const (
	VersionDraft      VersionStatus = "DRAFT"
	VersionPublished  VersionStatus = "PUBLISHED"
	VersionDeprecated VersionStatus = "DEPRECATED"
)

var (
	ErrInvalidContract        = errors.New("data contract is invalid")
	ErrInvalidContractVersion = errors.New("contract version is invalid")
	ErrInvalidTransition      = errors.New("contract version state transition is invalid")
)

type DataContract struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Code        string
	Name        string
	ProductCode string
	CreatedAt   time.Time
	CreatedBy   *uuid.UUID
}

type ContractVersion struct {
	ID            uuid.UUID
	ContractID    uuid.UUID
	MajorVersion  int
	MinorVersion  int
	PatchVersion  int
	Status        VersionStatus
	Document      map[string]any
	SourceRef     string
	SourceSHA256  string
	CreatedAt     time.Time
	CreatedBy     *uuid.UUID
	PublishedAt   *time.Time
	PublishedBy   *uuid.UUID
}

func NewDataContract(workspaceID uuid.UUID, code, name, productCode string, actorID *uuid.UUID) (DataContract, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	productCode = strings.TrimSpace(productCode)
	if workspaceID == uuid.Nil || code == "" || name == "" {
		return DataContract{}, ErrInvalidContract
	}
	return DataContract{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Code:        code,
		Name:        name,
		ProductCode: productCode,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   actorID,
	}, nil
}

func NewContractVersion(contractID uuid.UUID, majorVersion, minorVersion, patchVersion int, document map[string]any, sourceRef, sourceSHA256 string, actorID *uuid.UUID) (ContractVersion, error) {
	if contractID == uuid.Nil || majorVersion < 0 || minorVersion < 0 || patchVersion < 0 || len(document) == 0 || strings.TrimSpace(sourceSHA256) == "" {
		return ContractVersion{}, ErrInvalidContractVersion
	}
	return ContractVersion{
		ID:           uuid.New(),
		ContractID:   contractID,
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		PatchVersion: patchVersion,
		Status:       VersionDraft,
		Document:     document,
		SourceRef:    strings.TrimSpace(sourceRef),
		SourceSHA256: strings.TrimSpace(sourceSHA256),
		CreatedAt:    time.Now().UTC(),
		CreatedBy:    actorID,
	}, nil
}

func (v *ContractVersion) Publish(actorID *uuid.UUID) error {
	if v.Status != VersionDraft {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	v.Status = VersionPublished
	v.PublishedAt = &now
	v.PublishedBy = actorID
	return nil
}

func (v *ContractVersion) Deprecate(actorID *uuid.UUID) error {
	if v.Status != VersionPublished {
		return ErrInvalidTransition
	}
	v.Status = VersionDeprecated
	v.PublishedBy = actorID
	return nil
}

func (v ContractVersion) Semver() string {
	return fmtSemver(v.MajorVersion, v.MinorVersion, v.PatchVersion)
}

func fmtSemver(major, minor, patch int) string {
	return strings.Join([]string{itoa(major), itoa(minor), itoa(patch)}, ".")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for value > 0 {
		digits = append(digits, byte('0'+value%10))
		value /= 10
	}
	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}
	return string(digits)
}
