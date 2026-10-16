package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type remainingFlushBarrier struct {
	inner            *planeHTTPWriter
	once             sync.Once
	entered, release chan struct{}
}

func (w *remainingFlushBarrier) Write(b []byte) (int, error) { return w.WriteContext(w.inner.ctx, b) }
func (w *remainingFlushBarrier) WriteContext(ctx context.Context, b []byte) (int, error) {
	n, err := w.inner.WriteContext(ctx, b)
	if err == nil && n > 0 {
		w.once.Do(func() {
			close(w.entered)
			select {
			case <-w.release:
			case <-ctx.Done():
				err = ctx.Err()
			}
		})
	}
	return n, err
}

type remainingHTTPResult struct {
	Name      string
	Operation uuid.UUID
	Transfer  lc.FixtureReadResult
	Denial    string
	Error     string
}

func remainingTransfers(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, cs []domain.Copy, s remainingState, o *lc.OccupancyService, reader *lc.FixtureReaderService, pins *lc.FixtureSubjectPins, d *remainingDisposition) ([]remainingHTTPResult, error) {
	c := r.Base
	current := planeCurrentDeliveryGate(pool)
	direct := deliveryapp.NewDirectDataService(transaction.NewManager(pool), current.repo, current.gate, datasetinfra.NewPostgresRepository(pool)).WithTransferPins(pins)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	stop := func() { releaseOnce.Do(func() { close(release) }) }
	defer stop()
	result := make(chan remainingHTTPResult, 4)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name := "T1"
		cp := cs[2]
		profile := s.T1Profile
		if req.URL.Path == "/remaining/T2" {
			name = "T2"
			cp = cs[3]
			profile = s.T2Profile
		} else if req.URL.Path != "/remaining/T1" {
			w.WriteHeader(404)
			return
		}
		if req.Method != "POST" || req.ContentLength != 0 || req.URL.RawQuery != "" {
			w.WriteHeader(400)
			return
		}
		caller, e := (remainingAuthority{r}).Resolve(req, c.WorkspaceID, lc.CapabilityAdmitReader)
		if e != nil {
			w.WriteHeader(403)
			return
		}
		owned := lc.WithFixtureReaderCaller(req.Context(), caller)
		cmd := deliveryapp.DirectDataCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: cp.DatasetVersionID, ProfileID: profile, PrincipalRef: caller.Subject, EffectiveConsumerRef: pilot.PlaneConsumer, Purpose: pilot.PlanePurpose, Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: "remaining-http:" + name}
		issued, e := direct.Deliver(owned, cmd)
		rr := remainingHTTPResult{Name: name, Operation: issued.Operation.ID}
		if e != nil {
			if errors.Is(e, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
				rr.Denial = "SAME_KEY_REPLAY"
				w.WriteHeader(409)
			} else {
				var pgerr *pgconn.PgError
				if name == "T2" && errors.As(e, &pgerr) && pgerr.Code == "P0001" && pgerr.Message == "new fixture reference after restriction or disposition" {
					rr.Denial = "CURRENT_REFERENCE_FENCE"
					w.WriteHeader(403)
				} else {
					rr.Error = "unexpected direct gate failure"
					w.WriteHeader(500)
				}
			}
			result <- rr
			return
		}
		if !issued.PayloadReady || name != "T1" {
			rr.Error = "unexpected payload admission"
			w.WriteHeader(500)
			result <- rr
			return
		}
		output := &remainingFlushBarrier{inner: &planeHTTPWriter{ResponseWriter: w, ctx: owned}, entered: entered, release: release}
		rr.Transfer, e = reader.ReadPinnedTo(owned, c.WorkspaceID, issued.TransferPinID, output)
		if e != nil {
			rr.Error = e.Error()
		}
		result <- rr
	})}
	defer srv.Close()
	go srv.Serve(planeTCPListener{listener.(*net.TCPListener)})
	request := func(name string) (int, []byte, error) {
		req, e := http.NewRequestWithContext(ctx, "POST", "http://"+listener.Addr().String()+"/remaining/"+name, nil)
		if e != nil {
			return 0, nil, e
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		client := &http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }}
		resp, e := client.Do(req)
		if e != nil {
			return 0, nil, e
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
		return resp.StatusCode, b, e
	}
	payload := make(chan error, 1)
	go func() {
		status, b, e := request("T1")
		sum := sha256.Sum256(b)
		if e == nil && (status != 200 || int64(len(b)) != cs[2].ByteSize || hex.EncodeToString(sum[:]) != cs[2].ContentSHA256) {
			e = errors.New("T1 native TCP content mismatch")
		}
		payload <- e
	}()
	select {
	case <-entered:
	case e := <-payload:
		return nil, e
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err = d.restrict(ctx, cs[2]); err != nil {
		return nil, err
	}
	if err = d.approve(ctx, "T1"); err != nil {
		return nil, err
	}
	blocked, err := d.dispatch(ctx, "T1", "T1-after-real-flush", cs[2])
	if err != nil || blocked.Admitted {
		return nil, errors.New("T1 active writer admitted disposal")
	}
	if err = write(c, "remaining-T1-active-writer.json", map[string]any{"actual_native_GET_and_TCP_flush": true, "dispatch": blocked, "writer_joined": false, "at": time.Now().UTC()}); err != nil {
		return nil, err
	}
	stop()
	if err = <-payload; err != nil {
		return nil, err
	}
	first := <-result
	if first.Error != "" || !first.Transfer.StopConfirmed || first.Transfer.BytesWritten != cs[2].ByteSize {
		return nil, errors.New("T1 writer stop unconfirmed")
	}
	before, err := remainingNoIO(ctx, pool)
	if err != nil {
		return nil, err
	}
	status, b, err := request("T1")
	if err != nil || status != 409 || len(b) != 0 {
		return nil, errors.New("T1 same-key HTTP replay returned payload")
	}
	replay := <-result
	if err = d.restrict(ctx, cs[3]); err != nil {
		return nil, err
	}
	if err = d.approve(ctx, "T2"); err != nil {
		return nil, err
	}
	status, b, err = request("T2")
	if err != nil || status != 403 || len(b) != 0 {
		return nil, errors.New("T2 restriction-first HTTP returned payload")
	}
	denied := <-result
	after, err := remainingNoIO(ctx, pool)
	if err != nil || before != after || denied.Denial != "CURRENT_REFERENCE_FENCE" {
		return nil, errors.New("T1 replay/T2 created native I/O")
	}
	var issued bool
	if err = pool.QueryRow(ctx, `SELECT status='ISSUED' FROM delivery_operation WHERE id=$1`, first.Operation).Scan(&issued); err != nil || !issued {
		return nil, errors.New("T1 terminal history changed")
	}
	return []remainingHTTPResult{first, replay, denied}, nil
}
