package rightshttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type declarationPartyRequest struct {
	PartyRef string `json:"partyRef"`
	Role     string `json:"role"`
}
type declarationPermissionRequest struct {
	Kind      string `json:"kind"`
	Action    string `json:"action"`
	Purpose   string `json:"purpose"`
	ScopeType string `json:"scopeType"`
	ScopeRef  string `json:"scopeRef"`
}
type createDeclarationRequest struct {
	WorkspaceID       string                         `json:"workspaceId"`
	DataResourceID    string                         `json:"dataResourceId"`
	ClaimantRef       string                         `json:"claimantRef"`
	BasisType         string                         `json:"basisType"`
	BasisRef          string                         `json:"basisRef"`
	ConsumerScopeType string                         `json:"consumerScopeType"`
	ConsumerRef       string                         `json:"consumerRef"`
	EffectiveFrom     *time.Time                     `json:"effectiveFrom"`
	EffectiveTo       *time.Time                     `json:"effectiveTo"`
	Parties           []declarationPartyRequest      `json:"parties"`
	Permissions       []declarationPermissionRequest `json:"permissions"`
	Restrictions      map[string]any                 `json:"restrictions"`
	EvidenceIDs       []string                       `json:"evidenceIds"`
}

func (h *Handler) registerProvenance(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/rights-declarations", h.createRightsDeclaration)
	mux.HandleFunc("POST /api/v1/rights-declarations/{declarationId}/verify", h.verifyRightsDeclaration)
	mux.HandleFunc("POST /api/v1/rights-declarations/{declarationId}/reject", h.rejectRightsDeclaration)
	mux.HandleFunc("POST /api/v1/rights-declarations/{declarationId}/invalidate", h.invalidateRightsDeclaration)
	mux.HandleFunc("POST /api/v1/rights-declarations/{declarationId}/supersede", h.supersedeRightsDeclaration)
	mux.HandleFunc("POST /api/v1/authorizations/{authorizationId}/provenance-bindings", h.bindAuthorizationProvenance)
	mux.HandleFunc("POST /api/v1/authorization-provenance-bindings/{bindingId}/invalidate", h.invalidateAuthorizationProvenanceBinding)
	mux.HandleFunc("POST /api/v1/authorization-provenance-bindings/{bindingId}/supersede", h.supersedeAuthorizationProvenanceBinding)
	mux.HandleFunc("POST /api/v1/grantor-delegation-chains", h.createDelegationChain)
	mux.HandleFunc("POST /api/v1/grantor-delegation-chains/{chainId}/finalize", h.finalizeDelegationChain)
	mux.HandleFunc("POST /api/v1/grantor-delegation-chains/{chainId}/dispose", h.disposeDelegation)
	mux.HandleFunc("POST /api/v1/rights/entitlement-check", h.checkCurrentEntitlement)
	mux.HandleFunc("POST /api/v1/effective-rights", h.computeEffectiveRights)
	mux.HandleFunc("GET /api/v1/effective-rights/{snapshotId}", h.getEffectiveRights)
}

func (h *Handler) createDelegationChain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID         string `json:"workspaceId"`
		SourceDeclarationID string `json:"sourceDeclarationId"`
		Edges               []struct {
			ID                string     `json:"id"`
			Ordinal           int        `json:"ordinal"`
			DelegatorRef      string     `json:"delegatorRef"`
			DelegateRef       string     `json:"delegateRef"`
			DataResourceID    string     `json:"dataResourceId"`
			GrantableActions  []string   `json:"grantableActions"`
			GrantablePurposes []string   `json:"grantablePurposes"`
			ScopeType         string     `json:"scopeType"`
			ScopeRef          string     `json:"scopeRef"`
			ValidFrom         *time.Time `json:"validFrom"`
			ValidTo           *time.Time `json:"validTo"`
		} `json:"edges"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	workspace, e := uuid.Parse(body.WorkspaceID)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	source, e := uuid.Parse(body.SourceDeclarationID)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_RIGHTS_DECLARATION_ID", "sourceDeclarationId must be a UUID", nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, workspace)
	if !ok {
		return
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("grantor-delegation-chain-create:"+workspace.String()+":"+idempotencyKey))
	cmd := application.CreateDelegationChainCommand{WorkspaceID: workspace, SourceDeclarationID: source, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())}
	for index, value := range body.Edges {
		id, e := uuid.Parse(value.ID)
		if e != nil {
			id = uuid.NewSHA1(uuid.NameSpaceURL, []byte("grantor-delegation-edge:"+activityID.String()+":"+strconv.Itoa(index)))
		}
		resource, e := uuid.Parse(value.DataResourceID)
		if e != nil {
			httpserver.WriteError(w, r, 400, "INVALID_DATA_RESOURCE_ID", "dataResourceId must be a UUID", nil)
			return
		}
		scope, e := domain.NewNormalizedScope(value.ScopeType, value.ScopeRef)
		if e != nil {
			httpserver.WriteError(w, r, 400, "INVALID_SCOPE", "scopeType and scopeRef are required", nil)
			return
		}
		cmd.Edges = append(cmd.Edges, domain.DelegationEdge{ID: id, Ordinal: value.Ordinal, DelegatorRef: value.DelegatorRef, DelegateRef: value.DelegateRef, DataResourceID: resource, GrantableActions: value.GrantableActions, GrantablePurposes: value.GrantablePurposes, Scope: scope, ValidFrom: value.ValidFrom, ValidTo: value.ValidTo})
	}
	chain, e := h.service.CreateDelegationChain(r.Context(), cmd)
	if e != nil {
		httpserver.WriteError(w, r, 400, "DELEGATION_CHAIN_CREATE_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 201, chain)
}

func (h *Handler) finalizeDelegationChain(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "chainId", "INVALID_DELEGATION_CHAIN_ID")
	if !ok {
		return
	}
	chainRecord, e := h.repo.GetDelegationChain(r.Context(), id)
	if e != nil {
		httpserver.WriteError(w, r, 400, "DELEGATION_CHAIN_FINALIZE_FAILED", e.Error(), nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, chainRecord.WorkspaceID)
	if !ok {
		return
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("grantor-delegation-chain-finalize:"+id.String()+":"+idempotencyKey))
	chain, e := h.service.FinalizeDelegationChain(r.Context(), application.FinalizeDelegationChainCommand{ChainID: id, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if e != nil {
		httpserver.WriteError(w, r, 400, "DELEGATION_CHAIN_FINALIZE_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 200, chain)
}

func (h *Handler) disposeDelegation(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "chainId", "INVALID_DELEGATION_CHAIN_ID")
	if !ok {
		return
	}
	chainRecord, e := h.repo.GetDelegationChain(r.Context(), id)
	if e != nil {
		httpserver.WriteError(w, r, 400, "DELEGATION_DISPOSITION_FAILED", e.Error(), nil)
		return
	}
	var body struct {
		EdgeID      string     `json:"edgeId"`
		Disposition string     `json:"disposition"`
		EffectiveAt *time.Time `json:"effectiveAt"`
		Reason      string     `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	var edgeID *uuid.UUID
	if strings.TrimSpace(body.EdgeID) != "" {
		v, e := uuid.Parse(body.EdgeID)
		if e != nil {
			httpserver.WriteError(w, r, 400, "INVALID_EDGE_ID", "edgeId must be a UUID", nil)
			return
		}
		edgeID = &v
	}
	var at time.Time
	if body.EffectiveAt != nil {
		at = body.EffectiveAt.UTC()
	}
	actor, ok := h.authorizeWorkspace(w, r, chainRecord.WorkspaceID)
	if !ok {
		return
	}
	kind := strings.ToUpper(strings.TrimSpace(body.Disposition))
	edgeKey := "chain"
	if edgeID != nil {
		edgeKey = edgeID.String()
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("grantor-delegation-disposition:"+kind+":"+id.String()+":"+edgeKey+":"+idempotencyKey))
	d, e := h.service.DisposeDelegation(r.Context(), application.DisposeDelegationCommand{ChainID: id, EdgeID: edgeID, Disposition: kind, EffectiveAt: at, Reason: body.Reason, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if e != nil {
		httpserver.WriteError(w, r, 400, "DELEGATION_DISPOSITION_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 201, d)
}

func (h *Handler) createRightsDeclaration(w http.ResponseWriter, r *http.Request) {
	var req createDeclarationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspace, resource, ok := parseTwoUUIDs(w, r, req.WorkspaceID, req.DataResourceID, "workspaceId", "dataResourceId")
	if !ok {
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, workspace)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("rights-declaration-create:"+workspace.String()+":"+idempotencyKey))
	spec := domain.RightsDeclarationSpec{WorkspaceID: workspace, DataResourceID: resource, ClaimantRef: req.ClaimantRef, BasisType: req.BasisType, BasisRef: req.BasisRef, ConsumerScopeType: req.ConsumerScopeType, ConsumerRef: req.ConsumerRef, EffectiveFrom: req.EffectiveFrom, EffectiveTo: req.EffectiveTo, Restrictions: req.Restrictions, ActorID: actor}
	for _, value := range req.EvidenceIDs {
		evidenceID, parseErr := uuid.Parse(value)
		if parseErr != nil {
			httpserver.WriteError(w, r, 400, "INVALID_EVIDENCE_ID", "evidenceIds must contain UUIDs", nil)
			return
		}
		spec.EvidenceIDs = append(spec.EvidenceIDs, evidenceID)
	}
	for _, p := range req.Parties {
		spec.Parties = append(spec.Parties, domain.RightsParty{PartyRef: p.PartyRef, Role: p.Role})
	}
	for _, p := range req.Permissions {
		spec.Permissions = append(spec.Permissions, domain.RightsPermission{Kind: p.Kind, Action: p.Action, Purpose: p.Purpose, Scope: domain.NormalizedScope{Type: p.ScopeType, Ref: p.ScopeRef}})
	}
	d, err := h.service.CreateRightsDeclaration(r.Context(), application.CreateRightsDeclarationCommand{Spec: spec, ActivityID: &activityID, TraceID: httpserver.RequestID(r.Context())})
	if err != nil {
		httpserver.WriteError(w, r, 400, "RIGHTS_DECLARATION_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, 201, d)
}

func (h *Handler) verifyRightsDeclaration(w http.ResponseWriter, r *http.Request) {
	h.verifyDeclaration(w, r, domain.DeclarationVerified)
}
func (h *Handler) rejectRightsDeclaration(w http.ResponseWriter, r *http.Request) {
	h.verifyDeclaration(w, r, domain.DeclarationRejected)
}
func (h *Handler) verifyDeclaration(w http.ResponseWriter, r *http.Request, outcome string) {
	id, ok := parsePathUUID(w, r, "declarationId", "INVALID_RIGHTS_DECLARATION_ID")
	if !ok {
		return
	}
	declarationRecord, err := h.repo.GetRightsDeclaration(r.Context(), id)
	if err != nil {
		httpserver.WriteError(w, r, 400, "RIGHTS_DECLARATION_VERIFY_FAILED", err.Error(), nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, declarationRecord.WorkspaceID)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	var body struct {
		Reason     string `json:"reason"`
		EvidenceID string `json:"evidenceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	var evidenceID *uuid.UUID
	if strings.TrimSpace(body.EvidenceID) != "" {
		parsed, parseErr := uuid.Parse(body.EvidenceID)
		if parseErr != nil {
			httpserver.WriteError(w, r, 400, "INVALID_EVIDENCE_ID", "evidenceId must be a UUID", nil)
			return
		}
		evidenceID = &parsed
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("rights-declaration-verification:"+outcome+":"+id.String()+":"+idempotencyKey))
	v, err := h.service.VerifyRightsDeclaration(r.Context(), application.VerifyRightsDeclarationCommand{DeclarationID: id, Outcome: outcome, Reason: body.Reason, EvidenceID: evidenceID, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if err != nil {
		httpserver.WriteError(w, r, 400, "RIGHTS_DECLARATION_VERIFY_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, 201, v)
}

func (h *Handler) invalidateRightsDeclaration(w http.ResponseWriter, r *http.Request) {
	h.disposeDeclaration(w, r, domain.DispositionInvalidated)
}
func (h *Handler) supersedeRightsDeclaration(w http.ResponseWriter, r *http.Request) {
	h.disposeDeclaration(w, r, domain.DispositionSuperseded)
}
func (h *Handler) disposeDeclaration(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := parsePathUUID(w, r, "declarationId", "INVALID_RIGHTS_DECLARATION_ID")
	if !ok {
		return
	}
	declarationRecord, err := h.repo.GetRightsDeclaration(r.Context(), id)
	if err != nil {
		httpserver.WriteError(w, r, 400, "RIGHTS_DECLARATION_DISPOSITION_FAILED", err.Error(), nil)
		return
	}
	var body struct {
		Reason       string     `json:"reason"`
		EffectiveAt  *time.Time `json:"effectiveAt"`
		SupersededBy string     `json:"supersededByDeclarationId"`
		EvidenceID   string     `json:"evidenceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, declarationRecord.WorkspaceID)
	if !ok {
		return
	}
	var replacement *uuid.UUID
	if strings.TrimSpace(body.SupersededBy) != "" {
		v, e := uuid.Parse(body.SupersededBy)
		if e != nil {
			httpserver.WriteError(w, r, 400, "INVALID_SUPERSEDED_BY", "supersededByDeclarationId must be a UUID", nil)
			return
		}
		replacement = &v
	}
	var evidenceID *uuid.UUID
	if strings.TrimSpace(body.EvidenceID) != "" {
		v, e := uuid.Parse(body.EvidenceID)
		if e != nil {
			httpserver.WriteError(w, r, 400, "INVALID_EVIDENCE_ID", "evidenceId must be a UUID", nil)
			return
		}
		evidenceID = &v
	}
	var at time.Time
	if body.EffectiveAt != nil {
		at = body.EffectiveAt.UTC()
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("rights-declaration-disposition:"+kind+":"+id.String()+":"+idempotencyKey))
	d, err := h.service.DisposeRightsDeclaration(r.Context(), application.DisposeRightsDeclarationCommand{DeclarationID: id, Disposition: kind, EffectiveAt: at, Reason: body.Reason, SupersededBy: replacement, EvidenceID: evidenceID, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if err != nil {
		httpserver.WriteError(w, r, 400, "RIGHTS_DECLARATION_DISPOSITION_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, 201, d)
}

func (h *Handler) bindAuthorizationProvenance(w http.ResponseWriter, r *http.Request) {
	authID, ok := parsePathUUID(w, r, "authorizationId", "INVALID_AUTHORIZATION_ID")
	if !ok {
		return
	}
	var body struct {
		WorkspaceID         string     `json:"workspaceId"`
		DataResourceID      string     `json:"dataResourceId"`
		DeclarationID       string     `json:"rightsDeclarationId"`
		GrantorRef          string     `json:"grantorRef"`
		AuthorityMode       string     `json:"grantorAuthorityMode"`
		DelegationChainID   string     `json:"delegationChainId"`
		DelegationChainHash string     `json:"delegationChainHash"`
		AsOf                *time.Time `json:"asOf"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("rights-provenance-binding:"+idempotencyKey))
	workspace, resource, ok := parseTwoUUIDs(w, r, body.WorkspaceID, body.DataResourceID, "workspaceId", "dataResourceId")
	if !ok {
		return
	}
	declaration, e := uuid.Parse(body.DeclarationID)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_RIGHTS_DECLARATION_ID", "rightsDeclarationId must be a UUID", nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, workspace)
	if !ok {
		return
	}
	at := time.Now().UTC()
	if body.AsOf != nil {
		at = body.AsOf.UTC()
	}
	var chainID *uuid.UUID
	if strings.TrimSpace(body.DelegationChainID) != "" {
		parsed, parseErr := uuid.Parse(body.DelegationChainID)
		if parseErr != nil {
			httpserver.WriteError(w, r, 400, "INVALID_DELEGATION_CHAIN_ID", "delegationChainId must be a UUID", nil)
			return
		}
		chainID = &parsed
	}
	b, e := h.service.BindAuthorizationProvenance(r.Context(), application.BindAuthorizationProvenanceCommand{WorkspaceID: workspace, AuthorizationID: authID, DataResourceID: resource, DeclarationID: declaration, GrantorRef: body.GrantorRef, AuthorityMode: body.AuthorityMode, DelegationChainID: chainID, DelegationChainHash: body.DelegationChainHash, AsOf: at, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if e != nil {
		httpserver.WriteError(w, r, 400, "AUTHORIZATION_PROVENANCE_BIND_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 201, b)
}

func (h *Handler) invalidateAuthorizationProvenanceBinding(w http.ResponseWriter, r *http.Request) {
	h.disposeAuthorizationProvenanceBinding(w, r, domain.DispositionInvalidated)
}

func (h *Handler) supersedeAuthorizationProvenanceBinding(w http.ResponseWriter, r *http.Request) {
	h.disposeAuthorizationProvenanceBinding(w, r, domain.DispositionSuperseded)
}

func (h *Handler) disposeAuthorizationProvenanceBinding(w http.ResponseWriter, r *http.Request, kind string) {
	bindingID, ok := parsePathUUID(w, r, "bindingId", "INVALID_BINDING_ID")
	if !ok {
		return
	}
	var body struct {
		Reason       string     `json:"reason"`
		EffectiveAt  *time.Time `json:"effectiveAt"`
		SupersededBy string     `json:"supersededByBindingId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	var replacement *uuid.UUID
	if strings.TrimSpace(body.SupersededBy) != "" {
		v, err := uuid.Parse(body.SupersededBy)
		if err != nil {
			httpserver.WriteError(w, r, 400, "INVALID_SUPERSEDED_BY", "supersededByBindingId must be a UUID", nil)
			return
		}
		replacement = &v
	}
	var at time.Time
	if body.EffectiveAt != nil {
		at = body.EffectiveAt.UTC()
	}
	workspace, err := h.repo.GetBindingWorkspace(r.Context(), bindingID)
	if err != nil {
		httpserver.WriteError(w, r, 400, "BINDING_DISPOSITION_FAILED", err.Error(), nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, workspace)
	if !ok {
		return
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("authorization-provenance-binding-disposition:"+kind+":"+bindingID.String()+":"+idempotencyKey))
	d, err := h.service.DisposeAuthorizationProvenanceBinding(r.Context(), application.DisposeAuthorizationProvenanceBindingCommand{BindingID: bindingID, Disposition: kind, EffectiveAt: at, Reason: body.Reason, SupersededBy: replacement, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if err != nil {
		httpserver.WriteError(w, r, 400, "BINDING_DISPOSITION_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, 201, d)
}

func (h *Handler) checkCurrentEntitlement(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID     string     `json:"workspaceId"`
		AuthorizationID string     `json:"authorizationId"`
		DataResourceID  string     `json:"dataResourceId"`
		ConsumerRef     string     `json:"consumerRef"`
		Purpose         string     `json:"purpose"`
		Action          string     `json:"action"`
		ScopeType       string     `json:"scopeType"`
		ScopeRef        string     `json:"scopeRef"`
		AsOf            *time.Time `json:"asOf"`
		Path            string     `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspace, resource, ok := parseTwoUUIDs(w, r, body.WorkspaceID, body.DataResourceID, "workspaceId", "dataResourceId")
	if !ok {
		return
	}
	var auth uuid.UUID
	if strings.TrimSpace(body.AuthorizationID) != "" {
		var err error
		auth, err = uuid.Parse(body.AuthorizationID)
		if err != nil {
			httpserver.WriteError(w, r, 400, "INVALID_AUTHORIZATION_ID", "authorizationId must be a UUID", nil)
			return
		}
	}
	scope, e := domain.NewNormalizedScope(body.ScopeType, body.ScopeRef)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_SCOPE", "scopeType and scopeRef are required", nil)
		return
	}
	at := time.Now().UTC()
	if body.AsOf != nil {
		at = body.AsOf.UTC()
	}
	path := domain.EntitlementDownstream
	if strings.ToUpper(body.Path) == string(domain.EntitlementDirectUse) {
		path = domain.EntitlementDirectUse
	}
	decision, e := h.service.CheckCurrentEntitlement(r.Context(), application.CheckCurrentEntitlementCommand{EntitlementRequest: domain.EntitlementRequest{WorkspaceID: workspace, AuthorizationID: auth, DataResourceID: resource, ConsumerRef: body.ConsumerRef, Purpose: body.Purpose, Action: body.Action, Scope: scope, AsOf: at, Path: path}, TraceID: httpserver.RequestID(r.Context())})
	if e != nil {
		httpserver.WriteError(w, r, 500, "ENTITLEMENT_CHECK_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 200, decision)
}

func (h *Handler) computeEffectiveRights(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID            string     `json:"workspaceId"`
		TargetDatasetVersionID string     `json:"targetDatasetVersionId"`
		ConsumerRef            string     `json:"consumerRef"`
		Purpose                string     `json:"purpose"`
		AsOf                   *time.Time `json:"asOf"`
		EvidenceID             string     `json:"evidenceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	workspace, e := uuid.Parse(body.WorkspaceID)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	target, e := uuid.Parse(body.TargetDatasetVersionID)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_DATASET_VERSION_ID", "targetDatasetVersionId must be a UUID", nil)
		return
	}
	actor, ok := h.authorizeWorkspace(w, r, workspace)
	if !ok {
		return
	}
	at := time.Now().UTC()
	if body.AsOf != nil {
		at = body.AsOf.UTC()
	}
	var evidenceID *uuid.UUID
	if strings.TrimSpace(body.EvidenceID) != "" {
		parsed, parseErr := uuid.Parse(body.EvidenceID)
		if parseErr != nil {
			httpserver.WriteError(w, r, 400, "INVALID_EVIDENCE_ID", "evidenceId must be a UUID", nil)
			return
		}
		evidenceID = &parsed
	}
	activityID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("effective-rights-compute:"+workspace.String()+":"+idempotencyKey))
	snapshot, e := h.service.ComputeEffectiveRights(r.Context(), application.ComputeEffectiveRightsCommand{WorkspaceID: workspace, TargetDatasetVersionID: target, ConsumerRef: body.ConsumerRef, Purpose: body.Purpose, AsOf: at, AsOfProvided: body.AsOf != nil, EvidenceID: evidenceID, ActivityID: &activityID, ActorID: actor, TraceID: httpserver.RequestID(r.Context())})
	if e != nil {
		httpserver.WriteError(w, r, 400, "EFFECTIVE_RIGHTS_COMPUTE_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 201, snapshot)
}

func (h *Handler) getEffectiveRights(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "snapshotId", "INVALID_EFFECTIVE_RIGHTS_ID")
	if !ok {
		return
	}
	snapshot, e := h.repo.GetEffectiveRights(r.Context(), id)
	if errors.Is(e, infrastructure.ErrNotFound) {
		httpserver.WriteError(w, r, 404, "EFFECTIVE_RIGHTS_NOT_FOUND", "effective rights snapshot not found", nil)
		return
	}
	if e != nil {
		httpserver.WriteError(w, r, 500, "EFFECTIVE_RIGHTS_READ_FAILED", e.Error(), nil)
		return
	}
	writeJSON(w, 200, snapshot)
}

func parseTwoUUIDs(w http.ResponseWriter, r *http.Request, first, second, firstName, secondName string) (uuid.UUID, uuid.UUID, bool) {
	a, e := uuid.Parse(first)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_"+strings.ToUpper(firstName), firstName+" must be a UUID", nil)
		return uuid.Nil, uuid.Nil, false
	}
	b, e := uuid.Parse(second)
	if e != nil {
		httpserver.WriteError(w, r, 400, "INVALID_"+strings.ToUpper(secondName), secondName+" must be a UUID", nil)
		return uuid.Nil, uuid.Nil, false
	}
	return a, b, true
}
