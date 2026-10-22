package main

// Fixture-only barrier on the existing transfer SPI. Public delivery composition
// and its transaction/authorization semantics are unchanged.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type preIssuedCandidate struct {
	Operation, Pin uuid.UUID
	BackendPID     int
}
type preIssuedPause struct {
	inner   deliveryapp.DirectDataTransferPins
	entered chan preIssuedCandidate
	release chan struct{}
	crash   bool
	c       config
	before  map[string]int
}

func (p *preIssuedPause) AdmitDirectDataTransfer(ctx context.Context, tx pgx.Tx, op deliverydomain.Operation) (uuid.UUID, error) {
	pin, e := p.inner.AdmitDirectDataTransfer(ctx, tx, op)
	if e != nil {
		return uuid.Nil, e
	}
	candidate := preIssuedCandidate{Operation: op.ID, Pin: pin}
	if e = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&candidate.BackendPID); e != nil {
		return uuid.Nil, e
	}
	if p.crash {
		if e = write(p.c, "pre-issued-crash-candidate.json", map[string]any{"candidate": candidate, "before": p.before, "phase": "AFTER_REAL_PIN_ADMISSION_BEFORE_ISSUED_COMMIT", "native_get": 0, "payload_bytes": 0, "expected_exit": 94}); e != nil {
			return uuid.Nil, e
		}
		os.Exit(94) // Real process loss; PostgreSQL rolls back the operation/pin/history.
	}
	select {
	case p.entered <- candidate:
	case <-ctx.Done():
		return uuid.Nil, ctx.Err()
	}
	select {
	case <-p.release:
		return pin, nil
	case <-time.After(8 * time.Second):
		return uuid.Nil, errors.New("finite pre-ISSUED barrier expired")
	case <-ctx.Done():
		return uuid.Nil, ctx.Err()
	}
}
func preIssuedCounts(ctx context.Context, pool *pgxpool.Pool) (map[string]int, error) {
	result := map[string]int{}
	for _, t := range []string{"delivery_operation", "delivery_gate_evaluation", "delivery_transition", "lifecycle_reader_pin", "lifecycle_reader_termination", "lifecycle_storage_call", "lifecycle_storage_observation", "cost_event", "audit_event", "evidence", "outbox_event", "command_idempotency"} {
		var n int
		if e := pool.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); e != nil {
			return nil, e
		}
		result[t] = n
	}
	return result, nil
}
func preIssuedInvisible(ctx context.Context, pool *pgxpool.Pool, c preIssuedCandidate) (bool, error) {
	var visible bool
	e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_operation WHERE id=$1) OR EXISTS(SELECT 1 FROM lifecycle_reader_pin WHERE id=$2) OR EXISTS(SELECT 1 FROM outbox_event WHERE aggregate_id=$1) OR EXISTS(SELECT 1 FROM cost_allocation WHERE delivery_operation_id=$1)`, c.Operation, c.Pin).Scan(&visible)
	return !visible, e
}
func verifyPreIssuedCrash(ctx context.Context, c config, pool *pgxpool.Pool) error {
	var saved struct {
		Candidate preIssuedCandidate
		Before    map[string]int
	}
	b, e := os.ReadFile(filepath.Join(c.OutputDirectory, "pre-issued-crash-candidate.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &saved); e != nil {
		return e
	}
	invisible, e := preIssuedInvisible(ctx, pool, saved.Candidate)
	if e != nil || !invisible {
		return errors.New("crashed candidate facts committed")
	}
	after, e := preIssuedCounts(ctx, pool)
	if e != nil {
		return e
	}
	for t, n := range saved.Before {
		if after[t] != n {
			return fmt.Errorf("crash changed %s", t)
		}
	}
	return write(c, "pre-issued-crash-closure.json", map[string]any{"status": "PASS_REAL_PROCESS_ROLLBACK", "candidate": saved.Candidate, "before": saved.Before, "after": after, "native_get": 0, "payload_bytes": 0, "delete": 0})
}
func planePreIssued(ctx context.Context, c config, pool *pgxpool.Pool, original []domain.Copy, crash bool) error {
	state, e := loadPlaneState(c)
	if e != nil {
		return e
	}
	cs, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, policy, reader, pins, e := planeServices(ctx, c, pool, cs)
	if e != nil {
		return e
	}
	if _, e = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "pre-issued-beta-policy"}, DatasetVersionID: original[1].DatasetVersionID, UseUntil: c.ExpiresAt, RestoreUntil: c.ExpiresAt}); e != nil {
		return e
	}
	before, e := preIssuedCounts(ctx, pool)
	if e != nil {
		return e
	}
	current := planeCurrentDeliveryGate(pool)
	outcomes := []any{}
	for _, key := range []string{"pre-issued-positive", "pre-issued-fence-race", "pre-issued-restriction-first", "pre-issued-positive"} {
		pause := &preIssuedPause{inner: pins, entered: make(chan preIssuedCandidate, 1), release: make(chan struct{}), crash: crash, c: c, before: before}
		direct := deliveryapp.NewDirectDataService(transaction.NewManager(pool), current.repo, current.gate, datasetinfra.NewPostgresRepository(pool)).WithTransferPins(pause)
		done := make(chan map[string]any, 1)
		readAllowed := make(chan struct{})
		if key != "pre-issued-fence-race" {
			close(readAllowed)
		}
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller, e := authority{}.Resolve(r, c.WorkspaceID, lc.CapabilityAdmitReader)
			if e != nil || r.Method != "POST" || r.URL.Path != "/fixture/pre-issued" || r.ContentLength != 0 {
				http.Error(w, "denied", 403)
				done <- map[string]any{"error": "authority"}
				return
			}
			owned := lc.WithFixtureReaderCaller(r.Context(), caller)
			d, e := direct.Deliver(owned, deliveryapp.DirectDataCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: original[1].DatasetVersionID, ProfileID: state.ProfileID, PrincipalRef: caller.Subject, EffectiveConsumerRef: pilot.PlaneConsumer, Purpose: pilot.PlanePurpose, Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: key})
			if errors.Is(e, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
				http.Error(w, "new attempt required", 409)
				done <- map[string]any{"status": "REPLAY_NON_PAYLOAD", "operation": d.Operation.ID}
				return
			}
			if e != nil || !d.PayloadReady {
				http.Error(w, "blocked", 403)
				done <- map[string]any{"status": "BLOCKED", "operation": d.Operation.ID}
				return
			}
			<-readAllowed // Outside the committed transaction; fresh read gate still applies.
			out := &planeHTTPWriter{ResponseWriter: w, ctx: owned}
			result, e := reader.ReadPinnedTo(owned, c.WorkspaceID, d.TransferPinID, out)
			if e != nil && out.written == 0 {
				http.Error(w, "fresh read blocked", 403)
			}
			done <- map[string]any{"operation": d.Operation.ID, "pin": d.TransferPinID, "bytes": result.BytesWritten, "stop_confirmed": result.StopConfirmed, "error": fmt.Sprint(e)}
		})
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return e
		}
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second}
		go server.Serve(listener)
		conn, e := net.DialTimeout("tcp", listener.Addr().String(), 3*time.Second)
		if e != nil {
			server.Close()
			return e
		}
		fmt.Fprintf(conn, "POST /fixture/pre-issued HTTP/1.1\r\nHost: fixture\r\nAuthorization: Bearer %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", c.Token)
		precondition := map[string]any{"key": key}
		if key == "pre-issued-positive" && len(outcomes) == 0 || key == "pre-issued-fence-race" {
			var candidate preIssuedCandidate
			select {
			case candidate = <-pause.entered:
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Second):
				return errors.New("real transfer SPI not reached")
			}
			invisible, e := preIssuedInvisible(ctx, pool, candidate)
			if e != nil || !invisible {
				return errors.New("uncommitted candidate visible")
			}
			calls, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
			if e != nil || calls != before["lifecycle_storage_call"] {
				return errors.New("native I/O before ISSUED commit")
			}
			conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			one := make([]byte, 1)
			n, readErr := conn.Read(one)
			if n != 0 || readErr == nil {
				return errors.New("HTTP byte before ISSUED commit")
			}
			var timeout net.Error
			if !errors.As(readErr, &timeout) || !timeout.Timeout() {
				return errors.New("precommit TCP check was not a live timeout")
			}
			precondition["candidate"], precondition["committed_candidate_visible"], precondition["precommit_native_get"], precondition["precommit_tcp_bytes"] = candidate, false, 0, 0
			if key == "pre-issued-fence-race" {
				restricted := make(chan error, 1)
				go func() {
					_, e := o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "pre-issued-current-restriction"}, DatasetVersionID: original[1].DatasetVersionID, BasisRef: "controlled-pre-issued-fence-competitor"})
					restricted <- e
				}()
				waited := false
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					var blocked int
					if e = pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND wait_event_type='Lock'`, candidate.BackendPID).Scan(&blocked); e != nil {
						return e
					}
					if blocked > 0 {
						waited = true
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !waited {
					return errors.New("restriction did not wait on actual PostgreSQL fence")
				}
				precondition["postgresql_fence_wait_observed"] = true
				close(pause.release)
				select {
				case e = <-restricted:
					if e != nil {
						return e
					}
				case <-ctx.Done():
					return ctx.Err()
				}
				close(readAllowed)
			} else {
				close(pause.release)
			}
		}
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		response, e := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "POST"})
		if e != nil {
			return e
		}
		payload, e := io.ReadAll(io.LimitReader(response.Body, (128<<10)+1))
		response.Body.Close()
		conn.Close()
		server.Close()
		if e != nil {
			return e
		}
		result := <-done
		precondition["http_status"], precondition["result"] = response.StatusCode, result
		expected := 403
		if key == "pre-issued-positive" {
			expected = 200
			if len(outcomes) > 0 {
				expected = 409
			}
		}
		if response.StatusCode != expected {
			return fmt.Errorf("%s HTTP%d expected%d", key, response.StatusCode, expected)
		}
		after, e := preIssuedCounts(ctx, pool)
		if e != nil {
			return e
		}
		if expected == 200 {
			if !bytes.Equal(payload, pilot.PlaneSyntheticPayloads()[1]) || after["lifecycle_storage_call"] != before["lifecycle_storage_call"]+1 {
				return errors.New("positive committed native delivery differs")
			}
		} else if after["lifecycle_storage_call"] != before["lifecycle_storage_call"] {
			return errors.New("blocked/replay opened native data")
		}
		precondition["native_call_delta"] = after["lifecycle_storage_call"] - before["lifecycle_storage_call"]
		outcomes = append(outcomes, precondition)
		before = after
	}
	var issued int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM delivery_operation WHERE status='ISSUED' AND idempotency_key IN('pre-issued-positive','pre-issued-fence-race')`).Scan(&issued); e != nil || issued != 2 {
		return errors.New("original ISSUED histories changed")
	}
	return write(c, "pre-issued-native-closure.json", map[string]any{"status": "PASS_NATIVE_PRE_ISSUED_PAUSE", "cases": outcomes, "issued_history_retained": 2, "native_delete": 0, "root_mounted": false, "full_D1": "NOT_ACCEPTED"})
}
