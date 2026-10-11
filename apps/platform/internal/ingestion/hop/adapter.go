// Package hop implements the bounded ingestion Port using the existing Hop
// client, PostgreSQL driver and storage adapter. SQL/templates are configured
// by the server; a Command cannot supply SQL, URLs or credential values.
package hop

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	w "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	whop "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/hop"
)

type Table struct {
	Code, Schema, Name string
	Columns            []string
	Rows               int64
}
type Config struct {
	ConnectionRef, InstanceRef, Database, Reader, SourceDSN, RuntimeHash string
	Template                                                             []byte
	Metadata                                                             map[string][]json.RawMessage
	Tables                                                               []Table
}

// Identity deliberately excludes SourceDSN (which contains a secret). The
// public connection metadata, template and exact table contract are frozen.
func (c Config) Identity() (config, template, schema string) {
	probe := "INVALID"
	if p, err := pgx.ParseConfig(c.SourceDSN); err == nil {
		probe = fmt.Sprintf("%s:%d/%s/%s", p.Host, p.Port, p.Database, p.User)
	}
	b, _ := json.Marshal(struct {
		ConnectionRef, InstanceRef, Database, Reader, Probe, RuntimeHash string
		Metadata                                                         map[string][]json.RawMessage
	}{c.ConnectionRef, c.InstanceRef, c.Database, c.Reader, probe, c.RuntimeHash, c.Metadata})
	t, _ := json.Marshal(c.Tables)
	return d.Hash(b), d.Hash(c.Template), d.Hash(t)
}

type Adapter struct {
	Load                          func(context.Context) (Config, error)
	Client                        *whop.Client
	LocalRoot, EngineRoot, Bucket string
	// The publisher gets a fresh identity restricted to this exact attempt.
	Publisher func(context.Context, d.Operation) (*storage.Store, error)
}

func (*Adapter) Version() string { return "apache-hop/2.19.0;pgjdbc/42.7.14" }
func (a *Adapter) config(ctx context.Context, r d.Request) (Config, error) {
	c, err := a.Load(ctx)
	if err != nil {
		return c, d.ErrDenied
	}
	ch, th, sh := c.Identity()
	if c.ConnectionRef != r.ConnectionRef || c.InstanceRef != r.InstanceRef || ch != r.ConfigHash || th != r.TemplateHash || sh != r.SchemaHash || len(c.Tables) != 3 {
		return c, d.ErrDenied
	}
	var connection struct {
		RDBMS map[string]struct {
			Username, DatabaseName, Password string `json:",omitempty"`
		} `json:"rdbms"`
	}
	if len(c.Metadata["rdbms"]) != 1 || json.Unmarshal(c.Metadata["rdbms"][0], &connection) != nil {
		return c, d.ErrDenied
	}
	pg, ok := connection.RDBMS["POSTGRESQL"]
	if !ok || pg.Username != c.Reader || pg.DatabaseName != c.Database || pg.Password != "${SOURCE_DB_PASSWORD}" {
		return c, d.ErrDenied
	}
	parsed, err := pgx.ParseConfig(c.SourceDSN)
	if err != nil || parsed.Password == "" {
		return c, d.ErrDenied
	}
	meta, _ := json.Marshal(c.Metadata)
	if bytes.Contains(meta, []byte(parsed.Password)) || bytes.Contains(c.Template, []byte(parsed.Password)) {
		return c, d.ErrDenied
	}
	for _, s := range r.Slots {
		matched := false
		for _, t := range c.Tables {
			if t.Code == s.Code && slices.Equal(t.Columns, s.Columns) && t.Rows == s.Rows {
				matched = true
			}
		}
		if !matched {
			return c, d.ErrDenied
		}
	}
	return c, nil
}
func (a *Adapter) Check(ctx context.Context, r d.Request) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := a.config(ctx, r)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, c.SourceDSN)
	if err != nil {
		return d.ErrDenied
	}
	defer conn.Close(ctx)
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return d.ErrDenied
	}
	defer tx.Rollback(ctx)
	var safe bool
	err = tx.QueryRow(ctx, `SELECT current_database()=$1 AND current_user=$2 AND NOT rolsuper AND NOT rolinherit AND NOT rolcreaterole AND NOT rolcreatedb AND NOT rolreplication AND NOT rolbypassrls AND NOT has_database_privilege(current_database(),'CREATE') AND NOT has_database_privilege(current_database(),'TEMP') AND NOT has_schema_privilege('public','CREATE') AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=r.oid) FROM pg_roles r WHERE rolname=current_user`, c.Database, c.Reader).Scan(&safe)
	if err != nil || !safe {
		return d.ErrDenied
	}
	for _, t := range c.Tables {
		name := pgx.Identifier{t.Schema, t.Name}.Sanitize()
		err = tx.QueryRow(ctx, `SELECT has_schema_privilege($1,'USAGE') AND NOT has_schema_privilege($1,'CREATE') AND has_table_privilege($2,'SELECT') AND NOT has_table_privilege($2,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`, t.Schema, name).Scan(&safe)
		if err != nil || !safe {
			return d.ErrDenied
		}
		rows, err := tx.Query(ctx, `SELECT column_name,udt_name,is_nullable FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position`, t.Schema, t.Name)
		if err != nil {
			return d.ErrDenied
		}
		var columns []string
		for rows.Next() {
			var column, typ, nullable string
			if rows.Scan(&column, &typ, &nullable) != nil {
				rows.Close()
				return d.ErrDenied
			}
			if typ != "text" || nullable != "NO" {
				rows.Close()
				return d.ErrDenied
			}
			columns = append(columns, column)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !slices.Equal(columns, t.Columns) {
			return d.ErrDenied
		}
		var n int64
		if tx.QueryRow(ctx, "SELECT count(*) FROM "+name).Scan(&n) != nil || n != t.Rows {
			return d.ErrDenied
		}
	}
	return tx.Commit(ctx)
}
func name(op d.Operation) string { return "ingestion-" + op.AttemptID.String() }
func (a *Adapter) Register(ctx context.Context, op d.Operation) (string, error) {
	c, err := a.config(ctx, op.Request)
	if err != nil {
		return "", err
	}
	local := filepath.Join(a.LocalRoot, op.AttemptID.String())
	if err = os.MkdirAll(local, 0700); err != nil {
		return "", d.ErrUnknown
	}
	// Hop's container UID writes a private-parent, attempt-owned bind directory.
	if err = os.Chmod(local, 0777); err != nil {
		return "", d.ErrUnknown
	}
	pipeline := string(c.Template)
	start := strings.Index(pipeline, "<pipeline>")
	if start < 0 {
		return "", d.ErrManifest
	}
	pipeline = pipeline[start:]
	if strings.Count(pipeline, "<name>postgres-ingestion-v1</name>") != 1 {
		return "", d.ErrManifest
	}
	pipeline = strings.Replace(pipeline, "<name>postgres-ingestion-v1</name>", "<name>"+name(op)+"</name>", 1)
	meta, err := json.Marshal(c.Metadata)
	if err != nil {
		return "", d.ErrManifest
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, _ = gz.Write(meta)
	_ = gz.Close()
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(strings.TrimRight(a.EngineRoot, "/")+"/"+op.AttemptID.String()))
	definition := []byte("<pipeline_configuration>" + pipeline + "<pipeline_execution_configuration><parameters><parameter><name>OUTPUT_PREFIX</name><value>" + escaped.String() + "</value></parameter></parameters><run_configuration>local</run_configuration><log_level>Basic</log_level><clear_log>Y</clear_log><log_file>N</log_file></pipeline_execution_configuration><metastore_json>" + base64.StdEncoding.EncodeToString(compressed.Bytes()) + "</metastore_json></pipeline_configuration>")
	run, err := a.Client.PrepareSubmission(ctx, w.ManagedSubmitRequest{Name: name(op), Definition: definition, ContentType: "application/xml"})
	if err != nil {
		return "", d.ErrUnknown
	}
	return run.ID, nil
}
func (a *Adapter) Start(ctx context.Context, op d.Operation) error {
	_, err := a.Client.StartSubmission(ctx, w.ManagedSubmitRequest{Name: name(op)}, op.RemoteID)
	if err != nil {
		return d.ErrUnknown
	}
	return nil
}
func (a *Adapter) Status(ctx context.Context, op d.Operation) (string, error) {
	run, err := a.Client.Status(ctx, name(op), op.RemoteID)
	if err != nil || run.ID != op.RemoteID || run.Name != name(op) {
		return "UNKNOWN", d.ErrUnknown
	}
	switch run.State {
	case w.EngineRunSucceeded:
		return "SUCCEEDED", nil
	case w.EngineRunFailed:
		return "FAILED", nil
	case w.EngineRunRunning, w.EngineRunQueued:
		return "RUNNING", nil
	}
	return "UNKNOWN", d.ErrUnknown
}
func (a *Adapter) Manifest(ctx context.Context, op d.Operation) (d.Manifest, error) {
	r := op.Request
	m := d.Manifest{Contract: d.Contract, OperationID: op.ID, AttemptID: op.AttemptID, WorkspaceID: r.WorkspaceID, RequestKey: r.RequestKey, BatchKey: r.BatchKey, ConnectionRef: r.ConnectionRef, InstanceRef: r.InstanceRef, ConfigHash: r.ConfigHash, TemplateHash: r.TemplateHash, SchemaHash: r.SchemaHash, EngineVersion: a.Version(), RemoteID: op.RemoteID, ObservedAt: time.Now().UTC()}
	publisher, err := a.Publisher(ctx, op)
	if err != nil {
		return m, d.ErrUnknown
	}
	for _, s := range r.Slots {
		file, err := os.Open(filepath.Join(a.LocalRoot, op.AttemptID.String(), s.Code+".csv"))
		if err != nil {
			return m, d.ErrUnknown
		}
		data, err := io.ReadAll(io.LimitReader(file, d.MaxBytes+1))
		_ = file.Close()
		if err != nil {
			return m, d.ErrUnknown
		}
		out := d.Output{Code: s.Code, DatasetID: s.DatasetID, URI: d.Prefix(a.Bucket, op) + s.Code + ".csv", Bytes: int64(len(data)), Rows: s.Rows, SHA256: d.Hash(data), Columns: s.Columns}
		if d.ValidateContent(out, data) != nil {
			return m, d.ErrManifest
		}
		key := strings.TrimPrefix(out.URI, "s3://"+a.Bucket+"/")
		_, putErr := publisher.PutImmutable(ctx, key, bytes.NewReader(data), out.Bytes, "text/csv")
		// Recover a successful PUT whose response was lost; never replace bytes.
		reader, err := publisher.Get(ctx, out.URI)
		if err != nil {
			return m, d.ErrUnknown
		}
		saved, err := io.ReadAll(io.LimitReader(reader, d.MaxBytes+1))
		_ = reader.Close()
		if err != nil || d.ValidateContent(out, saved) != nil {
			return m, fmt.Errorf("staging verification failed")
		}
		_ = putErr
		m.Outputs = append(m.Outputs, out)
	}
	return m, nil
}
