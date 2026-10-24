package bindings

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"io"
	"net/http"
	"sort"
)

// AcceptancePrincipal is a private, versioned RP confirmation, not a command.
func (s *Store) AcceptancePrincipal(resolver principal.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		deny := func(status int) { http.Error(w, "identity authority unavailable", status) }
		if r.Method != "POST" {
			deny(405)
			return
		}
		if r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" || len(r.Header.Values("Authorization")) != 1 || r.Header.Get("Cookie") != "" {
			deny(400)
			return
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		t, err := d.Token()
		if err != nil || t != json.Delim('{') {
			deny(400)
			return
		}
		var raw string
		if !d.More() {
			deny(400)
			return
		}
		key, err := d.Token()
		if err != nil || key != "workspace_id" || d.Decode(&raw) != nil || d.More() {
			deny(400)
			return
		}
		if _, err = d.Token(); err != nil {
			deny(400)
			return
		}
		var trailing any
		if !errors.Is(d.Decode(&trailing), io.EOF) {
			deny(400)
			return
		}
		ws, err := uuid.Parse(raw)
		if err != nil || ws == uuid.Nil {
			deny(400)
			return
		}
		p, err := resolver.Resolve(r, ws, principal.CapabilityAcceptanceRead)
		if err != nil {
			if errors.Is(err, principal.ErrAuthorityUnavailable) {
				deny(503)
			} else if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
				deny(401)
			} else {
				deny(403)
			}
			return
		}
		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			deny(503)
			return
		}
		defer tx.Rollback(r.Context())
		a, err := s.ValidateInTx(r.Context(), tx, p, principal.CapabilityAcceptanceRead)
		if err != nil {
			if errors.Is(err, principal.ErrAuthorityUnavailable) {
				deny(503)
			} else {
				deny(403)
			}
			return
		}
		if tx.Commit(r.Context()) != nil {
			deny(503)
			return
		}
		caps := []string{}
		for _, c := range []string{principal.CapabilityAcceptanceRead, principal.CapabilityAcceptanceDraft, principal.CapabilityAcceptanceBasis, principal.CapabilityReceiptRecord} {
			if a.Principal.HasCapability(c) {
				caps = append(caps, c)
			}
		}
		sort.Strings(caps)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			ProfileVersion   string    `json:"profileVersion"`
			PrincipalID      uuid.UUID `json:"principalId"`
			CanonicalSubject string    `json:"canonicalSubject"`
			WorkspaceID      uuid.UUID `json:"workspaceId"`
			Issuer           string    `json:"issuer"`
			Subject          string    `json:"subject"`
			Revision         string    `json:"authorityRevision"`
			Capabilities     []string  `json:"capabilities"`
			ExpiresAt        any       `json:"expiresAt"`
		}{"business-acceptance.v1", p.ActorID, p.Subject, ws, p.IdentityIssuer, p.IdentitySubject, p.IdentityRevision, caps, a.ExpiresAt})
	}
}
