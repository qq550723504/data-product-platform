package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	pinnedworkflow "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/adapters/workflow"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	native "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

type planeFreshGate struct {
	gate *deliveryapp.CertificationDirectDataGate
	repo *deliveryinfra.PostgresRepository
}

func provisionProducer(ctx context.Context, c config, disable bool) error {
	if c.RootAccess == "" || c.RootSecret == "" || c.ProducerAccess == "" || c.ProducerSecret == "" {
		return lc.ErrStorageScope
	}
	a, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c})
	call := func(fn func(context.Context) error) error {
		x, can := context.WithTimeout(context.WithValue(ctx, adminKey{}, &adminCall{}), 15*time.Second)
		defer can()
		return fn(x)
	}
	if disable {
		return call(func(x context.Context) error { return a.SetUserStatus(x, c.ProducerAccess, madmin.AccountDisabled) })
	}
	p, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": []string{"s3:PutObject"}, "Resource": []string{"arn:aws:s3:::" + namespaces[0] + "/fixture/standardized.csv", "arn:aws:s3:::" + namespaces[0] + "/fixture/processed.csv"}}}})
	name := "lifecycle307-producer-" + c.RunID.String()
	if e = call(func(x context.Context) error { return a.AddCannedPolicy(x, name, p) }); e != nil {
		return e
	}
	if e = call(func(x context.Context) error { return a.AddUser(x, c.ProducerAccess, c.ProducerSecret) }); e != nil {
		return e
	}
	if e = call(func(x context.Context) error {
		_, e := a.AttachPolicy(x, madmin.PolicyAssociationReq{Policies: []string{name}, User: c.ProducerAccess})
		return e
	}); e != nil {
		return e
	}
	return write(c, "plane-producer-policy.json", json.RawMessage(p))
}
func planeCurrentDeliveryGate(pool *pgxpool.Pool) *planeFreshGate {
	tx := transaction.NewManager(pool)
	profiles := certificationinfra.NewProfileRepository(pool)
	cert := certificationapp.NewCertificationService(tx, profiles, certificationinfra.NewCertificationRepository(pool), certificationapp.NewReferenceEvidenceResolver())
	return &planeFreshGate{deliveryapp.NewCertificationDirectDataGate(certificationapp.NewEligibilityService(cert, datasetinfra.NewPostgresRepository(pool), rightsinfra.NewPostgresRepository(pool))), deliveryinfra.NewPostgresRepository(pool)}
}
func (g *planeFreshGate) ValidatePinnedSubject(ctx context.Context, tx pgx.Tx, pin domain.ReaderPin) error {
	if pin.Kind != domain.FixtureDelivery {
		return nil
	}
	op, e := g.repo.GetOperation(ctx, tx, *pin.DeliveryOperationID, false)
	if e != nil {
		return e
	}
	profile, e := g.repo.GetTerminalGateCertificationProfile(ctx, tx, op.ID)
	if e != nil {
		return e
	}
	r, e := g.gate.EvaluateDirectData(ctx, tx, deliveryapp.DirectDataGateRequest{OperationID: op.ID, WorkspaceID: op.WorkspaceID, DatasetVersionID: op.DatasetVersionID, ProfileID: profile, PrincipalRef: op.PrincipalRef, EffectiveConsumerRef: op.EffectiveConsumerRef, Purpose: op.Purpose, Action: op.Action, ScopeType: "ALL_RESOURCE", ScopeRef: op.DatasetVersionID.String(), DeliveryChannel: op.DeliveryChannel, DeliveryMode: op.DeliveryMode, RequestedExpiresAt: *op.FreshCapExpiresAt})
	if e != nil {
		return e
	}
	if !r.Evaluation.Allowed {
		return lc.ErrStorageDenied
	}
	return nil
}
func planeStep(ctx context.Context, mode string) error {
	c, e := load()
	if e != nil {
		return e
	}
	if !c.Plane {
		return lc.ErrStorageScope
	}
	m, cs, e := copies(c)
	if e != nil {
		return e
	}
	_ = m
	if mode == "provision" || mode == "seal" {
		if e = provision(ctx, c, cs, mode == "seal"); e != nil {
			return e
		}
		return provisionProducer(ctx, c, mode == "seal")
	}
	if c.RootAccess != "" || c.RootSecret != "" {
		return errors.New("plane owner cannot mount root")
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return e
	}
	defer pool.Close()
	var schema int
	if e = pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&schema); e != nil || schema != 68 {
		return errors.New("fresh isolated schema68 required")
	}
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	if mode == "setup" {
		_, e = planeSetup(ctx, c, pool, cs)
		return e
	}
	if mode == "accept" {
		return planeAccept(ctx, c, pool, cs)
	}
	if mode == "pre-issued" || mode == "pre-issued-crash" {
		return planePreIssued(ctx, c, pool, cs, mode == "pre-issued-crash")
	}
	if mode == "verify-pre-issued-crash" {
		return verifyPreIssuedCrash(ctx, c, pool)
	}
	if mode == "reference-races" && c.ReferenceDemand {
		return planeReferenceRaces(ctx, c, pool, cs)
	}
	if mode == "final" {
		return planeExport(ctx, c, pool)
	}
	return errors.New("unknown plane step")
}
func planeCopies(ctx context.Context, c config, pool *pgxpool.Pool) ([]domain.Copy, error) {
	rows, e := pool.Query(ctx, `SELECT c.id,c.dataset_version_id,n.namespace,c.object_key,c.provider_version,c.content_sha256,c.byte_size FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id WHERE c.workspace_id=$1 ORDER BY n.namespace,c.object_key`, c.WorkspaceID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var cs []domain.Copy
	for rows.Next() {
		cp := domain.Copy{WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID}
		if e = rows.Scan(&cp.ID, &cp.DatasetVersionID, &cp.Namespace, &cp.Key, &cp.ProviderVersion, &cp.ContentSHA256, &cp.ByteSize); e != nil {
			return nil, e
		}
		cs = append(cs, cp)
	}
	return cs, rows.Err()
}

type planeTCPListener struct{ *net.TCPListener }

func (l planeTCPListener) Accept() (net.Conn, error) {
	c, e := l.AcceptTCP()
	if e != nil {
		return nil, e
	}
	if e = c.SetWriteBuffer(1024); e != nil {
		c.Close()
		return nil, e
	}
	return c, nil
}

type planeHTTPWriter struct {
	http.ResponseWriter
	ctx     context.Context
	written int64
}

func (w *planeHTTPWriter) Write(b []byte) (int, error) { return w.WriteContext(w.ctx, b) }
func (w *planeHTTPWriter) WriteContext(ctx context.Context, b []byte) (int, error) {
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	rc := http.NewResponseController(w.ResponseWriter)
	if until, ok := ctx.Deadline(); ok {
		if e := rc.SetWriteDeadline(until); e != nil {
			return 0, e
		}
	}
	n, e := w.ResponseWriter.Write(b)
	w.written += int64(n)
	if e == nil {
		e = rc.Flush()
	}
	return n, e
}

type planeTransfer struct {
	Result      lc.FixtureReadResult
	Error       string
	At          time.Time
	Key         string
	OperationID uuid.UUID
}

func planeAccept(ctx context.Context, c config, pool *pgxpool.Pool, original []domain.Copy) error {
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
	until := time.Now().UTC().Add(45 * time.Second).Truncate(time.Microsecond)
	if _, e = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "plane-beta-natural-policy"}, DatasetVersionID: original[1].DatasetVersionID, UseUntil: until, RestoreUntil: c.ExpiresAt}); e != nil {
		return e
	}
	tx := transaction.NewManager(pool)
	wfRepo := workflowinfra.NewPostgresRepository(pool)
	execSvc := workflowapp.NewExecutionService(tx, wfRepo)
	writer, e := planeWriter(ctx, c, pool, o, pins, original, state)
	if e != nil {
		return e
	}
	engine := native.NewEngine("/run/repo/industry-packs", tx, datasetinfra.NewPostgresRepository(pool), entityinfra.NewPostgresRepository(pool), wfRepo, writer, planeCache{}, parkindicator.NewCalculator()).WithInputReader(pins)
	handler := workflowqueue.NewHandler(execSvc, wfRepo, &pinnedworkflow.PinnedEngine{Pins: pins, Inner: engine})
	create := func(key string) (workflowdomain.Execution, error) {
		return execSvc.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: c.WorkspaceID, WorkflowVersionID: state.WorkflowVersionID, OutputDatasetID: state.OutputDatasetID, TargetPeriod: pilot.PlanePeriod, Inputs: []workflowdomain.InputBinding{{Name: "enterprise_raw", DatasetVersionID: original[0].DatasetVersionID}, {Name: "enterprise_resolution", DatasetVersionID: state.StandardizedVersionID}, {Name: "lease_raw", DatasetVersionID: original[1].DatasetVersionID}, {Name: "energy_raw", DatasetVersionID: original[1].DatasetVersionID}}, ActorID: &c.ActorID, IdempotencyKey: key})
	}
	run := func(ex workflowdomain.Execution) error {
		b, _ := json.Marshal(map[string]any{"executionId": ex.ID})
		return handler.Handle(ctx, asynq.NewTask(workflowqueue.TaskExecute, b))
	}
	ex, e := create("plane-native-processing")
	if e != nil {
		return e
	}
	if e = run(ex); e != nil {
		return e
	}
	done, e := wfRepo.GetExecution(ctx, ex.ID)
	if e != nil {
		return e
	}
	if done.Status != workflowdomain.ExecutionSucceeded || done.OutputDatasetVersionID == nil {
		return fmt.Errorf("native processing did not succeed: %s %v", done.Status, done.ErrorMessage)
	}
	if e = write(c, "plane-native-processing.json", done); e != nil {
		return e
	}
	current := planeCurrentDeliveryGate(pool)
	direct := deliveryapp.NewDirectDataService(tx, current.repo, current.gate, datasetinfra.NewPostgresRepository(pool)).WithTransferPins(pins)
	results := make(chan planeTransfer, 8)
	var mu sync.Mutex
	ops := map[string]uuid.UUID{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/fixture/deliver" || r.ContentLength != 0 {
			http.Error(w, "invalid fixture delivery", 400)
			return
		}
		caller, e := authority{}.Resolve(r, c.WorkspaceID, lc.CapabilityAdmitReader)
		if e != nil {
			http.Error(w, "denied", 403)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key != "plane-delivery-complete" && key != "plane-delivery-slow" && key != "plane-delivery-expired" {
			http.Error(w, "unknown fixture attempt", 400)
			return
		}
		owned := lc.WithFixtureReaderCaller(r.Context(), caller)
		d, e := direct.Deliver(owned, deliveryapp.DirectDataCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: original[1].DatasetVersionID, ProfileID: state.ProfileID, PrincipalRef: caller.Subject, EffectiveConsumerRef: pilot.PlaneConsumer, Purpose: pilot.PlanePurpose, Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: key})
		if errors.Is(e, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
			results <- planeTransfer{Key: key, OperationID: d.Operation.ID, At: time.Now().UTC()}
			http.Error(w, "new delivery attempt required", 409)
			return
		}
		if e != nil {
			results <- planeTransfer{Error: e.Error(), Key: key}
			http.Error(w, "delivery denied", 403)
			return
		}
		mu.Lock()
		ops[key] = d.Operation.ID
		mu.Unlock()
		if !d.PayloadReady {
			results <- planeTransfer{Key: key, OperationID: d.Operation.ID, At: time.Now().UTC()}
			http.Error(w, "new attempt required", 409)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		out := &planeHTTPWriter{ResponseWriter: w, ctx: owned}
		res, e := reader.ReadPinnedTo(owned, c.WorkspaceID, d.TransferPinID, out)
		errText := ""
		if e != nil {
			errText = e.Error()
		}
		results <- planeTransfer{res, errText, time.Now().UTC(), key, d.Operation.ID}
		if e != nil && out.written == 0 {
			http.Error(w, "transfer blocked", 403)
		}
	})
	l, e := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if e != nil {
		return e
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 3 * time.Second}
	defer srv.Close()
	go srv.Serve(planeTCPListener{l})
	request := func(key string) (int, []byte, error) {
		r, _ := http.NewRequestWithContext(ctx, "POST", "http://"+l.Addr().String()+"/fixture/deliver", nil)
		r.Header.Set("Authorization", "Bearer "+c.Token)
		r.Header.Set("Idempotency-Key", key)
		response, e := (&http.Client{Timeout: 15 * time.Second}).Do(r)
		if e != nil {
			return 0, nil, e
		}
		defer response.Body.Close()
		b, e := io.ReadAll(io.LimitReader(response.Body, 128<<10))
		return response.StatusCode, b, e
	}
	status, b, e := request("plane-delivery-complete")
	if e != nil {
		return e
	}
	if status != 200 || !bytes.Equal(b, pilot.PlaneSyntheticPayloads()[1]) {
		failed := <-results
		_ = write(c, "plane-complete-failure.json", failed)
		return fmt.Errorf("complete delivery response mismatch: status %d: %s", status, failed.Error)
	}
	complete := <-results
	if !complete.Result.StopConfirmed || complete.Error != "" {
		return errors.New("complete delivery stop missing")
	}
	if e = write(c, "plane-delivery-complete.json", complete); e != nil {
		return e
	}
	callsBefore, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
	if e != nil {
		return e
	}
	status, _, e = request("plane-delivery-complete")
	if e != nil || status != 409 {
		return errors.New("same-key delivery replay did not refuse payload")
	}
	<-results
	callsAfter, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
	if e != nil || callsAfter != callsBefore {
		return errors.New("same-key replay reopened native data")
	}
	conn, e := net.DialTCP("tcp", nil, l.Addr().(*net.TCPAddr))
	if e != nil {
		return e
	}
	defer conn.Close()
	if e = conn.SetReadBuffer(1024); e != nil {
		return e
	}
	// Real TCP receiver stops reading entirely. No writer barrier and no fake deadline.
	raw := fmt.Sprintf("POST /fixture/deliver HTTP/1.1\r\nHost: fixture\r\nAuthorization: Bearer %s\r\nIdempotency-Key: plane-delivery-slow\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", c.Token)
	if _, e = io.WriteString(conn, raw); e != nil {
		return e
	}
	var active int
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reader_pin p JOIN delivery_operation o ON o.id=p.delivery_operation_id WHERE o.idempotency_key='plane-delivery-slow' AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)`).Scan(&active)
		if e != nil {
			return e
		}
		if active == 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if active != 1 {
		return errors.New("real slow TCP client did not keep transfer pin")
	}
	// Keep source restriction append-only; it blocks a new queued worker before I/O.
	restricted, e := create("plane-processing-restricted")
	if e != nil {
		return e
	}
	if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "plane-restrict-alpha"}, DatasetVersionID: original[0].DatasetVersionID, BasisRef: "Controlled reverse restriction before queued processing"}); e != nil {
		return e
	}
	before, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
	if e != nil {
		return e
	}
	if e = run(restricted); e != nil {
		return e
	}
	blocked, e := wfRepo.GetExecution(ctx, restricted.ID)
	if e != nil {
		return e
	}
	after, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
	if e != nil || before != after || blocked.Status != workflowdomain.ExecutionFailed {
		return errors.New("restricted worker performed I/O or did not fail")
	}
	if e = write(c, "plane-queued-restriction.json", map[string]any{"execution": blocked, "native_calls_before": before, "native_calls_after": after}); e != nil {
		return e
	}
	time.Sleep(500 * time.Millisecond)
	select {
	case early := <-results:
		return fmt.Errorf("TCP slow transfer stopped before natural expiry: %s", early.Error)
	default:
	}
	if e = write(c, "plane-slow-client-active.json", map[string]any{"active_transfer_pins": active, "observed_at": time.Now().UTC(), "use_until": until, "server_send_buffer": 1024, "client_receive_buffer": 1024, "client_payload_reads": 0, "writer_barrier": false}); e != nil {
		return e
	}
	var slow planeTransfer
	select {
	case slow = <-results:
	case <-ctx.Done():
		return ctx.Err()
	}
	if slow.Key != "plane-delivery-slow" || slow.Error == "" || !slow.Result.StopConfirmed || slow.At.Before(until) || slow.Result.BytesWritten >= original[1].ByteSize {
		return errors.New("natural expiry did not interrupt and confirm actual slow writer stop")
	}
	if e = write(c, "plane-slow-client-natural-expiry.json", slow); e != nil {
		return e
	}
	conn.Close()
	before, e = count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
	if e != nil {
		return e
	}
	status, _, e = request("plane-delivery-expired")
	if e != nil || status != 403 {
		return errors.New("naturally expired source admitted a new delivery")
	}
	<-results
	after, e = count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call`)
	if e != nil || before != after {
		return errors.New("expired delivery performed native I/O")
	}
	return write(c, "plane-acceptance-summary.json", map[string]any{"native_processing": "PASS", "direct_delivery": "PASS", "same_key_replay_zero_io": "PASS", "queued_reverse_restriction_zero_io": "PASS", "natural_expiry_actual_tcp_slow_client": "PASS", "expired_new_delivery_zero_io": "PASS", "use_until": until, "delete": 0, "cleanup": 0, "existing_protection": "unchanged", "production_acceptance": "NOT_RUN"})
}
func planeExport(ctx context.Context, c config, pool *pgxpool.Pool) error {
	cs, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, _, _, _, e := planeServices(ctx, c, pool, cs)
	if e != nil {
		return e
	}
	ins, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(o)})
	if e != nil {
		return e
	}
	var nativeFacts []any
	for _, cp := range cs {
		stat, e := ins.InspectExactCopy(ctx, cp)
		if e != nil {
			return e
		}
		pr, e := ins.InspectRetentionAndHold(ctx, cp)
		if e != nil {
			return e
		}
		nativeFacts = append(nativeFacts, map[string]any{"copy": cp, "stat_call_id": stat.CallID, "protection": pr})
	}
	var inventories []lc.NamespaceInventory
	seen := map[string]bool{}
	for _, cp := range cs {
		if seen[cp.Namespace] {
			continue
		}
		seen[cp.Namespace] = true
		inv, e := ins.InspectNamespace(ctx, cp)
		if e != nil {
			return e
		}
		inventories = append(inventories, inv)
	}
	if e = write(c, "plane-final-native-inspection.json", map[string]any{"objects": nativeFacts, "inventories": inventories, "delete": 0, "cleanup": 0}); e != nil {
		return e
	}
	queries := map[string]string{"pins": "SELECT row_to_json(p) FROM lifecycle_reader_pin p", "terminations": "SELECT row_to_json(p) FROM lifecycle_reader_termination p", "transfers": "SELECT row_to_json(p) FROM lifecycle_transfer_observation p", "calls": "SELECT row_to_json(p) FROM lifecycle_storage_call p", "observations": "SELECT row_to_json(p) FROM lifecycle_storage_observation p", "output-intents": "SELECT row_to_json(p) FROM lifecycle_processing_output_intent p", "outputs": "SELECT row_to_json(p) FROM lifecycle_processing_output_ready p", "inventory": "SELECT row_to_json(p) FROM (SELECT c.id,c.dataset_version_id,n.namespace,c.object_key,c.provider_version,c.content_sha256,c.byte_size FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id) p"}
	for name, q := range queries {
		rows, e := pool.Query(ctx, q)
		if e != nil {
			return e
		}
		var facts []json.RawMessage
		for rows.Next() {
			var b []byte
			if e = rows.Scan(&b); e != nil {
				rows.Close()
				return e
			}
			facts = append(facts, json.RawMessage(b))
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if e = write(c, "plane-final-"+name+".json", facts); e != nil {
			return e
		}
	}
	return nil
}
