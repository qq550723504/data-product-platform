package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"strings"
	"testing"
)

func TestRecoveredBindingCannotDowngradeFrozenControlledContract(t *testing.T) {
	frozen := d.SourceContract{AdmissionProtocol: d.ControlledSubmissionProtocol, ConnectionID: uuid.New(), ProviderIncarnation: "test", SourceCommit: strings.Repeat("1", 40), EngineVersion: "test", ImageDigest: "sha256:" + strings.Repeat("2", 64), NormalizerVersion: "label-studio-single-label-v1"}
	body, _ := json.Marshal(engineCampaignManifest{SourceContract: frozen})
	op := d.EngineOperation{OperationKind: d.EngineOperationEnsureCampaign, Provider: "test", ProviderInstanceRef: "test-instance", RequestID: "frozen-request", PayloadManifestHash: body}
	changed := frozen
	changed.ImageDigest = "sha256:" + strings.Repeat("3", 64)
	for _, contract := range []d.SourceContract{{}, changed} {
		binding := EngineCampaignBinding{SourceContract: contract, Provider: op.Provider, ProviderInstance: op.ProviderInstanceRef, RequestID: op.RequestID}
		// Reject before any repository write: an adapter response is not Core authority.
		s := &EngineService{}
		if err := s.persistEngineBindings(context.Background(), nil, op, engineResolution{CampaignBinding: &binding}); !errors.Is(err, d.ErrSourceIntegrity) {
			t.Fatalf("binding drift returned %v", err)
		}
	}
}
