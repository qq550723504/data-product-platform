package platformmanagement

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

const LogTimeLayout = "2006-01-02T15:04:05.000000000Z"

type RuntimeLogError string

func (e RuntimeLogError) Error() string { return string(e) }

const (
	LogInvalid       RuntimeLogError = "INVALID_LOG_QUERY"
	LogExpired       RuntimeLogError = "LOG_QUERY_EXPIRED"
	LogUnavailable   RuntimeLogError = "LOG_SOURCE_UNAVAILABLE"
	LogMissing       RuntimeLogError = "LOG_ENTRY_UNAVAILABLE"
	LogChanged       RuntimeLogError = "LOG_ENTRY_CHANGED"
	LogWindowExpired RuntimeLogError = "LOG_WINDOW_EXPIRED"
)

// RuntimeRecord is a normalized, allowlisted observation, not an AuditEvent.
// No free text, raw attributes, native URL, tenant or provider error is public.
type RuntimeRecord struct {
	ID                        string
	RecordedAt                time.Time
	OccurredAt                *time.Time
	Service, Level, EventCode string
}
type RuntimePosition struct {
	At time.Time
	ID string
}
type RuntimeQuery struct {
	Start, End, AsOf time.Time
	Service, Level   string
	After            *RuntimePosition
}
type RuntimePage struct {
	Items    []RuntimeRecord
	Next     *RuntimePosition
	Complete bool
}
type RuntimeReader interface {
	Query(context.Context, uuid.UUID, RuntimeQuery) (RuntimePage, error)
	ReadExact(context.Context, uuid.UUID, string) (RuntimeRecord, error)
}
type RuntimeBinding struct {
	ID, Version, RuleVersion, Source string
	Services                         []string
	Reader                           RuntimeReader
}
type RuntimeSources interface {
	Lookup(context.Context, uuid.UUID) (RuntimeBinding, bool)
}
type RuntimeGate interface {
	ConfirmRuntime(context.Context, principal.RequestPrincipal, bool) (principal.Authority, error)
}
type RuntimeLogHandler struct {
	Principal      principal.Resolver
	Gate           RuntimeGate
	Sources        RuntimeSources
	IdentitySource string
	codec          *securecookie.SecureCookie
	rowKey         []byte
	now            func() time.Time
}

// Keys are process-local: restart invalidates short-lived references. No new key
// service or reference ledger is created. SecureCookie owns encryption/MAC.
func NewRuntimeLogHandler(resolver principal.Resolver, gate RuntimeGate, sources RuntimeSources, identitySource string) (*RuntimeLogHandler, error) {
	key := make([]byte, 64)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	codec := securecookie.New(key[:32], key[32:]).SetSerializer(securecookie.JSONEncoder{}).MaxAge(900).MaxLength(4096)
	return &RuntimeLogHandler{resolver, gate, sources, identitySource, codec, key[:32], time.Now}, nil
}
func (h *RuntimeLogHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/workspaces/{workspaceId}/management/runtime-logs/{kind}", h.ServeHTTP)
	mux.HandleFunc("/api/v1/workspaces/{workspaceId}/management/runtime-logs/entries/{entryRef}", h.ServeHTTP)
}

type runtimeRef struct {
	Workspace, Caller                uuid.UUID
	Revision, Binding, Version, Rule string
	Start, End, AsOf                 time.Time
	Service, Level                   string
	Position                         RuntimePosition
	Hash                             string
	Expires                          time.Time
}
type runtimeEntry struct {
	RowKey     string  `json:"rowKey"`
	EntryRef   string  `json:"entryRef"`
	RecordedAt string  `json:"recordedAt"`
	OccurredAt *string `json:"occurredAt"`
	Service    string  `json:"service"`
	Level      string  `json:"level"`
	EventCode  string  `json:"eventCode"`
}
type runtimeWindow struct {
	Start     string `json:"start"`
	End       string `json:"end"`
	AsOf      string `json:"asOf"`
	TimeBasis string `json:"timeBasis"`
}

var logToken = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var logID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func validLogToken(s string) bool { return len(s) >= 16 && len(s) <= 2048 && logToken.MatchString(s) }
func logService(v string) bool    { return v == "APP_SERVICE" || v == "BACKGROUND_SERVICE" }
func logLevel(v string) bool {
	return slices.Contains([]string{"INFO", "WARN", "ERROR", "FATAL", "UNKNOWN"}, v)
}
func logEvent(v string) bool {
	return slices.Contains([]string{"READ_COMPLETED", "ACCESS_DENIED", "READ_FAILED", "DEPENDENCY_UNAVAILABLE", "UNCLASSIFIED"}, v)
}
func validRuntimeBinding(b RuntimeBinding) bool {
	if !logID.MatchString(b.ID) || !logID.MatchString(b.Version) || !logID.MatchString(b.RuleVersion) || b.Reader == nil || (b.Source != "CONFIGURED" && b.Source != "SYNTHETIC_FIXTURE") || len(b.Services) < 1 || len(b.Services) > 2 {
		return false
	}
	return slices.IsSorted(b.Services) && len(slices.Compact(slices.Clone(b.Services))) == len(b.Services) && slices.ContainsFunc(b.Services, logService) && !slices.ContainsFunc(b.Services, func(v string) bool { return !logService(v) })
}
func sameRuntimeBinding(a, b RuntimeBinding) bool {
	return a.ID == b.ID && a.Version == b.Version && a.RuleVersion == b.RuleVersion && a.Source == b.Source && slices.Equal(a.Services, b.Services) && validRuntimeBinding(a) == validRuntimeBinding(b) && (a.Reader == nil) == (b.Reader == nil)
}
func (h *RuntimeLogHandler) lookup(ctx context.Context, ws uuid.UUID) (RuntimeBinding, bool) {
	if h.Sources == nil {
		return RuntimeBinding{}, false
	}
	b, ok := h.Sources.Lookup(ctx, ws)
	b.Services = slices.Clone(b.Services)
	return b, ok
}
func logAuthorityFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, principal.ErrCapabilityDenied) || errors.Is(err, principal.ErrWorkspaceDenied) {
		failure(w, 403, "LOG_ACCESS_DENIED")
	} else if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		failure(w, 401, "AUTHENTICATION_REQUIRED")
	} else {
		failure(w, 503, string(LogUnavailable))
	}
}
func logFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, LogInvalid):
		failure(w, 400, string(LogInvalid))
	case errors.Is(err, LogExpired):
		failure(w, 409, string(LogExpired))
	case errors.Is(err, LogChanged):
		failure(w, 409, string(LogChanged))
	case errors.Is(err, LogMissing):
		failure(w, 404, string(LogMissing))
	case errors.Is(err, LogWindowExpired):
		failure(w, 410, string(LogWindowExpired))
	default:
		failure(w, 503, string(LogUnavailable))
	}
}
func parseLogTime(raw string) (time.Time, error) {
	t, e := time.Parse(LogTimeLayout, raw)
	if e != nil || t.UTC().Format(LogTimeLayout) != raw {
		return time.Time{}, LogInvalid
	}
	return t.UTC(), nil
}
func logQuery(values url.Values, now time.Time) (RuntimeQuery, string, error) {
	q := RuntimeQuery{AsOf: now.UTC()}
	for k, v := range values {
		if !slices.Contains([]string{"start", "end", "service", "level", "cursor"}, k) || len(v) != 1 || v[0] == "" {
			return q, "", LogInvalid
		}
	}
	var e error
	q.Start, e = parseLogTime(values.Get("start"))
	if e != nil {
		return q, "", e
	}
	q.End, e = parseLogTime(values.Get("end"))
	if e != nil || !q.Start.Before(q.End) || q.End.Sub(q.Start) > 24*time.Hour || q.End.After(now) {
		return q, "", LogInvalid
	}
	q.Service = values.Get("service")
	q.Level = values.Get("level")
	if q.Service != "" && !logService(q.Service) || q.Level != "" && !logLevel(q.Level) {
		return q, "", LogInvalid
	}
	c := values.Get("cursor")
	if c != "" && !validLogToken(c) {
		return q, "", LogInvalid
	}
	return q, c, nil
}
func (h *RuntimeLogHandler) decode(value, kind string, p principal.RequestPrincipal, b RuntimeBinding) (runtimeRef, error) {
	var ref runtimeRef
	if !validLogToken(value) {
		return ref, LogInvalid
	}
	padded := value + strings.Repeat("=", (4-len(value)%4)%4)
	if h.codec == nil || h.codec.Decode("runtime-logs."+kind+".v1", padded, &ref) != nil {
		return ref, LogExpired
	}
	if ref.Caller != p.ActorID || ref.Workspace != p.WorkspaceID {
		return ref, principal.ErrWorkspaceDenied
	}
	if ref.Revision != p.IdentityRevision || ref.Binding != b.ID || ref.Version != b.Version || ref.Rule != b.RuleVersion || !ref.Expires.After(h.now()) {
		return ref, LogExpired
	}
	if !ref.Start.Before(ref.End) || ref.End.Sub(ref.Start) > 24*time.Hour || ref.AsOf.After(h.now()) || !logID.MatchString(ref.Position.ID) || ref.Position.At.Before(ref.Start) || !ref.Position.At.Before(ref.End) || ref.Position.At.After(ref.AsOf) || ref.Service != "" && !logService(ref.Service) || ref.Level != "" && !logLevel(ref.Level) {
		return ref, LogExpired
	}
	return ref, nil
}
func (h *RuntimeLogHandler) encode(ref runtimeRef, kind string) (string, error) {
	s, e := h.codec.Encode("runtime-logs."+kind+".v1", ref)
	s = strings.TrimRight(s, "=")
	if e != nil || !validLogToken(s) {
		return "", LogUnavailable
	}
	return s, nil
}
func (h *RuntimeLogHandler) stableKey(ws uuid.UUID, b RuntimeBinding, id string) string {
	m := hmac.New(sha256.New, h.rowKey)
	_, _ = m.Write([]byte("runtime-row.v1\x00" + ws.String() + "\x00" + b.ID + "\x00" + b.Version + "\x00" + id))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func runtimeHash(r RuntimeRecord) string {
	b, _ := json.Marshal(struct {
		At                    string
		Occurred              *time.Time
		Service, Level, Event string
	}{r.RecordedAt.UTC().Format(LogTimeLayout), r.OccurredAt, r.Service, r.Level, r.EventCode})
	s := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(s[:])
}
func validRuntimeShape(r RuntimeRecord, b RuntimeBinding) bool {
	return logID.MatchString(r.ID) && !r.RecordedAt.IsZero() && r.RecordedAt.Location() == time.UTC && (r.OccurredAt == nil || !r.OccurredAt.IsZero() && r.OccurredAt.Location() == time.UTC) && slices.Contains(b.Services, r.Service) && logLevel(r.Level) && logEvent(r.EventCode)
}
func validRuntimeRecord(r RuntimeRecord, b RuntimeBinding, q RuntimeQuery) bool {
	return validRuntimeShape(r, b) && !r.RecordedAt.Before(q.Start) && r.RecordedAt.Before(q.End) && !r.RecordedAt.After(q.AsOf) && (q.Service == "" || q.Service == r.Service) && (q.Level == "" || q.Level == r.Level)
}
func beforePosition(r RuntimeRecord, pos RuntimePosition) bool {
	return r.RecordedAt.Before(pos.At) || r.RecordedAt.Equal(pos.At) && r.ID < pos.ID
}
func validRuntimePage(page RuntimePage, b RuntimeBinding, q RuntimeQuery) bool {
	if !page.Complete || len(page.Items) > 25 || !ValidRuntimeBatch(page.Items, b, q) {
		return false
	}
	if page.Next != nil {
		if len(page.Items) != 25 {
			return false
		}
		last := page.Items[24]
		return page.Next.At.Equal(last.RecordedAt) && page.Next.ID == last.ID
	}
	return true
}

// ValidRuntimeBatch checks the entire normalized source batch, including the
// optional 26th lookahead record. Adapters must call this before truncating a
// batch into a public page; an invalid lookahead cannot prove another page.
func ValidRuntimeBatch(items []RuntimeRecord, b RuntimeBinding, q RuntimeQuery) bool {
	if items == nil || len(items) > 26 {
		return false
	}
	seen := map[string]bool{}
	for i, r := range items {
		if !validRuntimeRecord(r, b, q) || seen[r.ID] || q.After != nil && !beforePosition(r, *q.After) || i > 0 && !beforePosition(r, RuntimePosition{items[i-1].RecordedAt, items[i-1].ID}) {
			return false
		}
		seen[r.ID] = true
	}
	return true
}
func sameLogPrincipal(a, b principal.RequestPrincipal) bool {
	return a.ActorID == b.ActorID && a.Subject == b.Subject && a.WorkspaceID == b.WorkspaceID && a.IdentityIssuer == b.IdentityIssuer && a.IdentitySubject == b.IdentitySubject && a.IdentityRevision == b.IdentityRevision
}
func (h *RuntimeLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" {
		failure(w, 405, "READ_ONLY")
		return
	}
	ws, e := uuid.Parse(r.PathValue("workspaceId"))
	if e != nil || ws == uuid.Nil || r.ContentLength > 0 || len(r.TransferEncoding) > 0 || r.Header.Get("Cookie") != "" || len(r.Header.Values("Authorization")) != 1 {
		logFailure(w, LogInvalid)
		return
	}
	detail := r.PathValue("entryRef")
	kind := r.PathValue("kind")
	if detail != "" {
		kind = "entries"
	}
	records := kind == "entries"
	if !records && kind != "source" {
		failure(w, 404, "NOT_FOUND")
		return
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || (kind == "source" || detail != "") && len(values) > 0 {
		logFailure(w, LogInvalid)
		return
	}
	var q RuntimeQuery
	var cursor string
	if records && detail == "" {
		q, cursor, e = logQuery(values, h.now())
		if e != nil {
			logFailure(w, e)
			return
		}
	}
	if h.Principal == nil || h.Gate == nil {
		logAuthorityFailure(w, principal.ErrNotConfigured)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	p, e := h.Principal.Resolve(r, ws, principal.CapabilityManagementRead)
	if e != nil {
		logAuthorityFailure(w, e)
		return
	}
	if p.WorkspaceID != ws {
		logAuthorityFailure(w, principal.ErrWorkspaceDenied)
		return
	}
	a, e := h.Gate.ConfirmRuntime(ctx, p, records)
	if e != nil {
		logAuthorityFailure(w, e)
		return
	}
	binding, configured := h.lookup(ctx, ws)
	state := "UNCONFIGURED"
	if configured {
		state = "CONFIGURED"
		if !validRuntimeBinding(binding) {
			state = "UNSUPPORTED"
		}
	}
	if records && state != "CONFIGURED" {
		failure(w, 503, "LOG_SOURCE_"+state)
		return
	}
	var page RuntimePage
	var record RuntimeRecord
	var ref runtimeRef
	var readErr error
	if records {
		if detail != "" {
			ref, e = h.decode(detail, "entry", p, binding)
			if e == nil {
				q = RuntimeQuery{Start: ref.Start, End: ref.End, AsOf: ref.AsOf, Service: ref.Service, Level: ref.Level}
				record, e = binding.Reader.ReadExact(ctx, ws, ref.Position.ID)
				if e == nil && (record.ID != ref.Position.ID || !validRuntimeShape(record, binding)) {
					e = LogUnavailable
				}
				if e == nil && runtimeHash(record) != ref.Hash {
					e = LogChanged
				}
				if e == nil && !validRuntimeRecord(record, binding, q) {
					e = LogUnavailable
				}
			}
		} else {
			if cursor != "" {
				ref, e = h.decode(cursor, "cursor", p, binding)
				if e == nil {
					if !q.Start.Equal(ref.Start) || !q.End.Equal(ref.End) || q.Service != ref.Service || q.Level != ref.Level {
						e = LogInvalid
					} else {
						q.AsOf = ref.AsOf
						q.After = &ref.Position
					}
				}
			}
			if e == nil {
				page, e = binding.Reader.Query(ctx, ws, q)
				if e == nil && !validRuntimePage(page, binding, q) {
					e = LogUnavailable
				}
			}
		}
		// Source errors are observations too. Do not disclose existence/change or
		// failure facts until the same post-I/O authority/binding gate has passed.
		readErr = e
	}
	fresh, e := h.Principal.Resolve(r, ws, principal.CapabilityManagementRead)
	if e != nil {
		logAuthorityFailure(w, e)
		return
	}
	if !sameLogPrincipal(p, fresh) {
		logAuthorityFailure(w, principal.ErrUnauthenticated)
		return
	}
	if fresh.IdentityExpiresAt.Before(p.IdentityExpiresAt) {
		p.IdentityExpiresAt = fresh.IdentityExpiresAt
	}
	a, e = h.Gate.ConfirmRuntime(ctx, p, records)
	if e != nil {
		logAuthorityFailure(w, e)
		return
	}
	current, still := h.lookup(ctx, ws)
	if configured != still || configured && !sameRuntimeBinding(binding, current) {
		logFailure(w, LogExpired)
		return
	}
	if ctx.Err() != nil || !a.ExpiresAt.After(h.now()) {
		logFailure(w, LogUnavailable)
		return
	}
	if !ref.Expires.IsZero() && !ref.Expires.After(h.now()) {
		logFailure(w, LogExpired)
		return
	}
	if readErr != nil {
		if errors.Is(readErr, principal.ErrWorkspaceDenied) {
			logAuthorityFailure(w, readErr)
		} else {
			logFailure(w, readErr)
		}
		return
	}
	var data any
	source := h.IdentitySource
	if source != "CONFIGURED" && source != "SYNTHETIC_FIXTURE" {
		logFailure(w, LogUnavailable)
		return
	}
	if !records {
		data = struct {
			State        string `json:"state"`
			Availability string `json:"availability"`
		}{state, "NOT_CHECKED"}
	} else {
		source = binding.Source
		expires := minTime(h.now().Add(15*time.Minute), a.ExpiresAt)
		if !ref.Expires.IsZero() {
			expires = minTime(expires, ref.Expires)
		}
		contextRef := runtimeRef{Workspace: ws, Caller: p.ActorID, Revision: p.IdentityRevision, Binding: binding.ID, Version: binding.Version, Rule: binding.RuleVersion, Start: q.Start, End: q.End, AsOf: q.AsOf, Service: q.Service, Level: q.Level, Expires: expires}
		project := func(v RuntimeRecord) (runtimeEntry, error) {
			entry := contextRef
			entry.Position = RuntimePosition{v.RecordedAt, v.ID}
			entry.Hash = runtimeHash(v)
			token, err := h.encode(entry, "entry")
			var occurred *string
			if v.OccurredAt != nil {
				s := v.OccurredAt.UTC().Format(LogTimeLayout)
				occurred = &s
			}
			return runtimeEntry{h.stableKey(ws, binding, v.ID), token, v.RecordedAt.UTC().Format(LogTimeLayout), occurred, v.Service, v.Level, v.EventCode}, err
		}
		window := runtimeWindow{q.Start.Format(LogTimeLayout), q.End.Format(LogTimeLayout), q.AsOf.Format(LogTimeLayout), "OBSERVED"}
		if detail != "" {
			item, err := project(record)
			if err != nil {
				logFailure(w, err)
				return
			}
			item.EntryRef = detail
			data = struct {
				Window runtimeWindow `json:"window"`
				Item   runtimeEntry  `json:"item"`
			}{window, item}
		} else {
			items := make([]runtimeEntry, 0, len(page.Items))
			for _, item := range page.Items {
				v, err := project(item)
				if err != nil {
					logFailure(w, err)
					return
				}
				items = append(items, v)
			}
			var next *string
			if page.Next != nil {
				c := contextRef
				c.Position = *page.Next
				token, err := h.encode(c, "cursor")
				if err != nil {
					logFailure(w, err)
					return
				}
				next = &token
			}
			data = struct {
				Window runtimeWindow  `json:"window"`
				Items  []runtimeEntry `json:"items"`
				Next   *string        `json:"next"`
			}{window, items, next}
		}
	}
	bytes, err := json.Marshal(struct {
		Envelope
		Data any `json:"data"`
	}{Envelope{"platform-management.v1", ws, p.ActorID, a.Principal.IdentityRevision, a.EvaluatedAt, source, false}, data})
	limit := 256 << 10
	if detail != "" {
		limit = 64 << 10
	}
	if err != nil || len(bytes) > limit || ctx.Err() != nil {
		logFailure(w, LogUnavailable)
		return
	}
	// Serialization is still inside the same read: never release records with
	// an expired authority or query reference, and never renew that reference.
	if !a.ExpiresAt.After(h.now()) {
		logFailure(w, LogUnavailable)
		return
	}
	if !ref.Expires.IsZero() && !ref.Expires.After(h.now()) {
		logFailure(w, LogExpired)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(bytes)
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
