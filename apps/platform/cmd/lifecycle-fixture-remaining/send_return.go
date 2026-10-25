package main

import (
	"errors"
	"fmt"

	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

type sendReturn struct {
	Execution                  string                       `json:"execution"`
	Stage                      string                       `json:"stage"`
	Target                     domain.Copy                  `json:"target"`
	Attempt                    domain.ProviderAttempt       `json:"attempt"`
	FailurePhase               string                       `json:"failure_phase,omitempty"`
	Certainty                  string                       `json:"certainty,omitempty"`
	Outcome                    string                       `json:"outcome,omitempty"`
	SendError                  bool                         `json:"send_error"`
	BudgetConsumed             bool                         `json:"budget_consumed"`
	ClaimConfirmed             bool                         `json:"claim_confirmed"`
	ProviderObservationPresent bool                         `json:"provider_observation_present"`
	ProviderInvocation         string                       `json:"provider_invocation"`
	TransportObservation       *domain.ProviderObservation  `json:"transport_observation,omitempty"`
	HTTPResponse               *storage.ExactDeleteResponse `json:"http_response,omitempty"`
	AutomaticResend            bool                         `json:"automatic_resend"`
	AutomaticRecovery          bool                         `json:"automatic_recovery"`
}

func writePreSendRejection(path, stage string, attempt domain.ProviderAttempt, target domain.Copy, phase string, cause error) error {
	// This local receipt is not a Core NOT_SENT or provider observation fact.
	r := sendReturn{Execution: execution, Stage: stage, Target: target, Attempt: attempt,
		FailurePhase: phase, SendError: true, ProviderInvocation: "NOT_ATTEMPTED"}
	return errors.Join(cause, save(path, r))
}

// Keep the original claim error before the SDK's normalized adapter error and
// never query for a provider observation when the transport refused its claim.
func finishSendReturn(path, stage string, attempt domain.ProviderAttempt, target domain.Copy, j *journal, sendErr error, lookup func() (string, string, error)) error {
	r := sendReturn{Execution: execution, Stage: stage, Target: target, Attempt: attempt,
		SendError: sendErr != nil, BudgetConsumed: j.budgetConsumed, ClaimConfirmed: j.claimConfirmed,
		ProviderInvocation: "UNKNOWN", TransportObservation: j.transportObservation}
	var failure *storage.ExactDeleteFailure
	if errors.As(sendErr, &failure) {
		r.HTTPResponse = &failure.Response
		if failure.Response.HTTPStatus == 401 || failure.Response.HTTPStatus == 403 {
			r.FailurePhase = "EXACT_HTTP_RESPONSE_DENIED"
		}
	}
	if j.transportObservation != nil && j.transportObservation.InvokedAt != nil {
		r.ProviderInvocation = domain.InvocationConfirmed
	}
	if j.claimError != nil {
		r.FailurePhase, r.ProviderInvocation = "PRE_SEND_CLAIM_REJECTED", "NOT_ATTEMPTED"
		return errors.Join(fmt.Errorf("exact send claim: %w", j.claimError), sendErr, save(path, r))
	}
	certainty, outcome, err := lookup()
	if err != nil {
		r.FailurePhase = "PROVIDER_OBSERVATION_UNAVAILABLE"
		return errors.Join(sendErr, j.observationError, fmt.Errorf("read persisted provider observation: %w", err), save(path, r))
	}
	r.ProviderObservationPresent, r.Certainty, r.Outcome = true, certainty, outcome
	if certainty == domain.InvocationConfirmed {
		r.ProviderInvocation = domain.InvocationConfirmed
	}
	if j.observationError != nil {
		// A successful read after an uncertain write does not erase the write
		// failure or the transport's original proof, even for expected UNKNOWN.
		r.FailurePhase, r.SendError = "PROVIDER_OBSERVATION_PERSISTENCE_FAILED", true
		return errors.Join(j.observationError, sendErr, save(path, r))
	}
	if err = save(path, r); err != nil {
		return errors.Join(sendErr, err)
	}
	if certainty != domain.InvocationConfirmed {
		return errors.Join(sendErr, lc.ErrStorageUnknown)
	}
	if stage == "H2" || stage == "M1B" {
		if outcome != domain.ProviderUnknown {
			return errors.Join(sendErr, lc.ErrStorageContract)
		}
		// Expected response loss advances only the separately selected read-only
		// path. The original CONFIRMED UNKNOWN and send error remain in r.
		return nil
	}
	if outcome != domain.ProviderAccepted {
		return errors.Join(sendErr, lc.ErrStorageContract)
	}
	return sendErr
}
