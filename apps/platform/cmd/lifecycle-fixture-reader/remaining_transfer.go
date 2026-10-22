package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	inner   *planeHTTPWriter
	once    sync.Once
	entered chan struct{}
}

func (w *remainingFlushBarrier) Write(b []byte) (int, error) { return w.WriteContext(w.inner.ctx, b) }
func (w *remainingFlushBarrier) WriteContext(ctx context.Context, b []byte) (int, error) {
	// Flush a real prefix, then enter the remaining real TCP write. There is no
	// synthetic pause: the client's deliberately small receive window blocks it.
	n, err := w.inner.WriteContext(ctx, b[:256])
	if err != nil {
		return n, err
	}
	w.once.Do(func() { close(w.entered) })
	m, err := w.inner.WriteContext(ctx, b[256:])
	return n + m, err
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
	entered := make(chan struct{})
	cancelOwner := make(chan context.CancelFunc, 1)
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
		ownerCtx, ownerCancel := context.WithCancel(req.Context())
		defer ownerCancel()
		owned := lc.WithFixtureReaderCaller(ownerCtx, caller)
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
		cancelOwner <- ownerCancel
		output := &remainingFlushBarrier{inner: &planeHTTPWriter{ResponseWriter: w, ctx: owned}, entered: entered}
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
	connection, err := net.DialTCP("tcp", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	if err = connection.SetReadBuffer(1024); err != nil {
		return nil, err
	}
	if err = connection.SetDeadline(time.Now().UTC().Add(time.Minute)); err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintf(connection, "POST /remaining/T1 HTTP/1.1\r\nHost: fixture\r\nAuthorization: Bearer %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", c.Token); err != nil {
		return nil, err
	}
	select {
	case <-entered:
	case <-result:
		return nil, errors.New("T1 exited before real TCP flush")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-result:
		return nil, errors.New("T1 socket did not actually block")
	case <-time.After(200 * time.Millisecond):
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
	ownerCancel := <-cancelOwner
	ownerCancel()
	select {
	case <-result:
		return nil, errors.New("T1 cancel returned while socket should remain blocked")
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var activePins int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reader_pin p WHERE p.copy_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)`, cs[2].ID).Scan(&activePins); err != nil || activePins != 1 {
		return nil, errors.New("T1 cancellation prematurely unpinned writer")
	}
	if err = write(c, "remaining-T1-active-writer.json", map[string]any{"actual_native_GET_and_TCP_prefix_flush": true, "real_socket_backpressure": true, "cancelled_owner_still_blocked_and_pinned": true, "dispatch": blocked, "writer_joined": false, "at": time.Now().UTC()}); err != nil {
		return nil, err
	}
	// Only now drain the real socket, allowing Write+Flush to return and the
	// reader service to record its confirmed termination/transfer observation.
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	response.Body.Close()
	sum := sha256.Sum256(body)
	if err != nil || response.StatusCode != 200 || int64(len(body)) != cs[2].ByteSize || hex.EncodeToString(sum[:]) != cs[2].ContentSHA256 {
		return nil, errors.New("T1 native TCP content mismatch")
	}
	first := <-result
	if first.Error != context.Canceled.Error() || !first.Transfer.StopConfirmed || first.Transfer.BytesWritten != cs[2].ByteSize {
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
