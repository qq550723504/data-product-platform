package domain

import "time"

var AcceptanceRoles = []string{"PROVIDER", "RECEIVER", "BASIS_CONFIRMER", "REVIEWER", "ARCHIVIST", "DISPOSER"}

// AcceptanceBindingFact is an immutable business fact, never an IAM permission.
// Optional references belong to its explicit Kind; no inferred/latest binding exists.
type AcceptanceBindingFact struct {
	ID                 string     `json:"id"`
	WorkspaceID        string     `json:"workspaceId"`
	Kind               string     `json:"kind"`
	BasisEvidenceID    string     `json:"basisEvidenceId"`
	RecordedBy         string     `json:"recordedBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	Origin             string     `json:"origin,omitempty"`
	ValidFrom          time.Time  `json:"validFrom"`
	ValidTo            *time.Time `json:"validTo"`
	ActorID            string     `json:"actorId,omitempty"`
	Subject            string     `json:"subject,omitempty"`
	PartyRef           string     `json:"partyRef,omitempty"`
	DeliveryID         string     `json:"deliveryOperationId,omitempty"`
	DatasetVersionID   string     `json:"datasetVersionId,omitempty"`
	Role               string     `json:"role,omitempty"`
	RepresentationID   string     `json:"representationId,omitempty"`
	ContractVersionID  string     `json:"contractVersionId,omitempty"`
	SourceSHA256       string     `json:"sourceSha256,omitempty"`
	CriteriaSetHash    string     `json:"criteriaSetHash,omitempty"`
	ConfirmerBindingID string     `json:"confirmerBindingId,omitempty"`
	TargetID           string     `json:"targetId,omitempty"`
	TargetKind         string     `json:"targetKind,omitempty"`
	Disposition        string     `json:"disposition,omitempty"`
	EffectiveAt        *time.Time `json:"effectiveAt,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	State              string     `json:"state"`
}
type AcceptanceResponsibility struct {
	Role            string                 `json:"role"`
	Status          string                 `json:"status"`
	Reason          string                 `json:"reason"`
	CandidateIDs    []string               `json:"candidateIds"`
	Binding         *AcceptanceBindingFact `json:"binding"`
	Representative  *AcceptanceBindingFact `json:"representative"`
	IsCurrentCaller bool                   `json:"isCurrentCaller"`
}
type AcceptanceApplicability struct {
	Status       string                 `json:"status"`
	Reason       string                 `json:"reason"`
	CandidateIDs []string               `json:"candidateIds"`
	Binding      *AcceptanceBindingFact `json:"binding"`
}
type AcceptanceBindingContext struct {
	WorkspaceID, DeliveryID, DatasetVersionID, ConsumerRef             string
	ActorID, Subject, ContractVersionID, SourceSHA256, CriteriaSetHash string
	StandardSupported                                                  bool
	At                                                                 time.Time
}

// EvaluateAcceptanceBindings uses the complete scoped fact set at one database
// transaction time. History pagination must never change the current decision.
func EvaluateAcceptanceBindings(c AcceptanceBindingContext, facts []AcceptanceBindingFact) ([]AcceptanceResponsibility, AcceptanceApplicability, []AcceptanceBindingFact) {
	byID := map[string]AcceptanceBindingFact{}
	key := func(kind, id string) string { return kind + ":" + id }
	for _, f := range facts {
		if f.WorkspaceID == c.WorkspaceID {
			byID[key(f.Kind, f.ID)] = f
		}
	}
	baseState := func(f AcceptanceBindingFact) string {
		if f.Kind == "DISPOSITION" {
			if f.EffectiveAt != nil && !c.At.Before(*f.EffectiveAt) {
				return "EFFECTIVE"
			}
			return "SCHEDULED"
		}
		for _, d := range byID {
			if d.Kind == "DISPOSITION" && d.TargetKind == f.Kind && d.TargetID == f.ID && d.EffectiveAt != nil && !c.At.Before(*d.EffectiveAt) {
				return "DISPOSED"
			}
		}
		if c.At.Before(f.ValidFrom) {
			return "NOT_YET_VALID"
		}
		if f.ValidTo != nil && !c.At.Before(*f.ValidTo) {
			return "EXPIRED"
		}
		return "ACTIVE"
	}
	for id, f := range byID {
		f.State = baseState(f)
		byID[id] = f
	}
	roles := make([]AcceptanceResponsibility, 0, len(AcceptanceRoles))
	for _, role := range AcceptanceRoles {
		r := AcceptanceResponsibility{Role: role, Status: "UNASSIGNED", Reason: "NO_BINDING", CandidateIDs: []string{}}
		exists := false
		for _, f := range facts {
			if f.WorkspaceID == c.WorkspaceID && f.Kind == "RESPONSIBILITY" && f.DeliveryID == c.DeliveryID && f.DatasetVersionID == c.DatasetVersionID && f.Role == role {
				exists = true
				if byID[key(f.Kind, f.ID)].State == "ACTIVE" {
					r.CandidateIDs = append(r.CandidateIDs, f.ID)
				}
			}
		}
		if exists {
			r.Status = "INACTIVE"
			r.Reason = "NO_CURRENT_BINDING"
		}
		if len(r.CandidateIDs) > 1 {
			r.Status = "AMBIGUOUS"
			r.Reason = "MULTIPLE_CURRENT_BINDINGS"
		}
		if len(r.CandidateIDs) == 1 {
			f := byID[key("RESPONSIBILITY", r.CandidateIDs[0])]
			r.Binding = &f
			rep, ok := byID[key("REPRESENTATION", f.RepresentationID)]
			if ok && rep.Kind == "REPRESENTATION" {
				r.Representative = &rep
			}
			if !ok || rep.Kind != "REPRESENTATION" || rep.State != "ACTIVE" {
				r.Status = "INACTIVE"
				r.Reason = "REPRESENTATION_NOT_CURRENT"
			} else if (role == "RECEIVER" || role == "BASIS_CONFIRMER" || role == "REVIEWER") && rep.PartyRef != c.ConsumerRef {
				r.Status = "INACTIVE"
				r.Reason = "RECEIVING_PARTY_MISMATCH"
			} else {
				r.Status = "ASSIGNED"
				r.Reason = "CURRENT_BINDING"
				r.IsCurrentCaller = rep.ActorID == c.ActorID && rep.Subject == c.Subject
			}
		}
		roles = append(roles, r)
	}
	provider := roles[0]
	for _, i := range []int{2, 3} {
		if provider.Status == "ASSIGNED" && roles[i].Status == "ASSIGNED" && (provider.Representative.ActorID == roles[i].Representative.ActorID || provider.Representative.Subject == roles[i].Representative.Subject) {
			roles[i].Status = "CONFLICT"
			roles[i].Reason = "PROVIDER_SEPARATION_REQUIRED"
		}
	}
	app := AcceptanceApplicability{Status: "UNSELECTED", Reason: "EXPLICIT_STANDARD_REQUIRED", CandidateIDs: []string{}}
	if c.ContractVersionID != "" {
		app.Status = "MISSING"
		app.Reason = "NO_APPLICABILITY_BINDING"
		for _, f := range facts {
			if f.WorkspaceID == c.WorkspaceID && f.Kind == "APPLICABILITY" && f.DeliveryID == c.DeliveryID && f.DatasetVersionID == c.DatasetVersionID && f.ContractVersionID == c.ContractVersionID {
				if app.Status == "MISSING" {
					app.Status = "INACTIVE"
					app.Reason = "NO_CURRENT_APPLICABILITY"
				}
				if byID[key(f.Kind, f.ID)].State == "ACTIVE" {
					app.CandidateIDs = append(app.CandidateIDs, f.ID)
				}
			}
		}
		if len(app.CandidateIDs) > 1 {
			app.Status = "AMBIGUOUS"
			app.Reason = "MULTIPLE_CURRENT_APPLICABILITY"
		}
		if len(app.CandidateIDs) == 1 {
			f := byID[key("APPLICABILITY", app.CandidateIDs[0])]
			app.Binding = &f
			app.Status = "MISMATCH"
			app.Reason = "SOURCE_OR_CONFIRMER_MISMATCH"
			confirmer := roles[2]
			if c.StandardSupported && f.SourceSHA256 == c.SourceSHA256 && f.CriteriaSetHash == c.CriteriaSetHash && confirmer.Status == "ASSIGNED" && confirmer.Binding.ID == f.ConfirmerBindingID {
				app.Status = "MATCHED"
				app.Reason = "EXACT_SOURCE_AND_CONFIRMER"
			}
		}
	}
	history := make([]AcceptanceBindingFact, 0, len(facts))
	for _, f := range facts {
		if f.WorkspaceID != c.WorkspaceID {
			continue
		}
		f = byID[key(f.Kind, f.ID)]
		if f.Kind == "RESPONSIBILITY" && f.State == "ACTIVE" {
			for _, r := range roles {
				if r.Binding != nil && r.Binding.ID == f.ID && r.Status != "ASSIGNED" {
					f.State = r.Status
					f.Reason = r.Reason
				}
			}
		}
		if f.Kind == "APPLICABILITY" && app.Binding != nil && app.Binding.ID == f.ID && app.Status != "MATCHED" {
			f.State = app.Status
			f.Reason = app.Reason
		}
		history = append(history, f)
	}
	return roles, app, history
}
