// Package sourceadmission admits bounded, approved external source evidence.
// It does not implement Annotation Result/Review or Dataset certification.
package sourceadmission

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const MaxBytes = 32 << 20
const Capability = "INGEST_RAW"

var (
	ErrDenied      = errors.New("SOURCE_AUTHORITY_DENIED")
	ErrConflict    = errors.New("SOURCE_IDEMPOTENCY_CONFLICT")
	ErrIntegrity   = errors.New("SOURCE_INTEGRITY_FAILED")
	ErrUnsupported = errors.New("SOURCE_PROFILE_UNSUPPORTED")
	ErrUnknown     = errors.New("SOURCE_TRANSPORT_UNKNOWN")
)

//go:embed release-manifest-v1.schema.json
var schemaBytes []byte
var manifestSchema = compileSchema()

func compileSchema() *jsonschema.Schema {
	c := jsonschema.NewCompiler()
	var v any
	if json.Unmarshal(schemaBytes, &v) != nil {
		panic("invalid embedded source schema")
	}
	if err := c.AddResource("urn:fork:release-manifest:v1", v); err != nil {
		panic(err)
	}
	s, err := c.Compile("urn:fork:release-manifest:v1")
	if err != nil {
		panic(err)
	}
	return s
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

var decimal = regexp.MustCompile(`^[1-9][0-9]*$`)
var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Source struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	ID        string `json:"id"`
}
type Member struct {
	Source     Source            `json:"source"`
	Project    string            `json:"project_id"`
	Task       string            `json:"task_id"`
	Assignment string            `json:"assignment_id"`
	Revision   string            `json:"revision"`
	ResultHash string            `json:"result_hash"`
	RecordHash string            `json:"record_sha256"`
	Media      []json.RawMessage `json:"media"`
}
type Asset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Format string `json:"format"`
	SHA    string `json:"sha256"`
	Size   int64  `json:"size_bytes"`
	Count  int    `json:"record_count"`
}
type Manifest struct {
	Snapshot      string `json:"snapshot_id"`
	Mode          string `json:"origin_mode"`
	ContentDigest string `json:"content_digest"`
	Content       struct {
		Namespace    string `json:"source_namespace"`
		Project      string `json:"project_id"`
		Organization string `json:"organization_id"`
		Mapping      string `json:"native_mapping_version"`
		MediaProfile string `json:"media_profile"`
		XMLHash      string `json:"label_config_sha256"`
		SchemaHash   string `json:"schema_sha256"`
		Output       struct {
			Format string `json:"format"`
		} `json:"output"`
		Items []Member `json:"items"`
	} `json:"content"`
	Receipt struct {
		ID string `json:"release_id"`
	} `json:"receipt"`
	Artifacts []Asset           `json:"artifacts"`
	Media     []json.RawMessage `json:"media"`
	Policy    struct {
		Mode      string `json:"mode"`
		Namespace string `json:"source_namespace"`
	} `json:"access_policy"`
}
type Bundle struct {
	Manifest Manifest
	Blobs    map[string][]byte
}

// strictJSON rejects duplicate keys, invalid UTF-8, non-JSON numbers and trailing
// values. UseNumber preserves legacy number lexemes; native bytes are never JCS.
func strictJSON(b []byte, target any) error {
	if !utf8.Valid(b) {
		return ErrIntegrity
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		if z, ok := t.(json.Delim); ok {
			switch z {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return e
					}
					key, ok := k.(string)
					if !ok || seen[key] {
						return ErrIntegrity
					}
					seen[key] = true
					if e = walk(); e != nil {
						return e
					}
				}
				x, e := d.Token()
				if e != nil || x != json.Delim('}') {
					return ErrIntegrity
				}
			case '[':
				for d.More() {
					if e := walk(); e != nil {
						return e
					}
				}
				x, e := d.Token()
				if e != nil || x != json.Delim(']') {
					return ErrIntegrity
				}
			default:
				return ErrIntegrity
			}
		}
		return nil
	}
	if walk() != nil {
		return ErrIntegrity
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrIntegrity
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(target) != nil {
		return ErrIntegrity
	}
	return nil
}
func ValidateManifest(blob []byte, c Connection, r Request) (Manifest, error) {
	var m Manifest
	if len(blob) == 0 || len(blob) > MaxBytes || Hash(blob) != r.ManifestSHA || strictJSON(blob, &m) != nil {
		return m, ErrIntegrity
	}
	canonical, err := jsoncanonicalizer.Transform(blob)
	if err != nil || !bytes.Equal(canonical, blob) {
		return m, ErrIntegrity
	}
	var value any
	if json.Unmarshal(blob, &value) != nil || manifestSchema.Validate(value) != nil {
		return m, ErrIntegrity
	}
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(blob, &envelope)
	content, err := jsoncanonicalizer.Transform(envelope["content"])
	if err != nil || Hash(content) != m.ContentDigest {
		return m, ErrIntegrity
	}
	if m.Mode != "fork_approved" || m.Policy.Mode != "fork_approved" || c.Mode != "fork_approved" {
		return m, ErrDenied
	}
	if m.Snapshot != r.SnapshotID.String() || m.Receipt.ID != r.ReleaseID.String() || m.Content.Namespace != c.Namespace.String() || m.Policy.Namespace != c.Namespace.String() || m.Content.Project != c.Project || m.Content.Organization != c.Organization {
		return m, ErrIntegrity
	}
	if m.Content.Mapping != "submission-envelope/v1" || m.Content.MediaProfile != "no-media/v1" || len(m.Media) != 0 || m.Content.Output.Format != "JSONL" {
		return m, ErrUnsupported
	}
	if len(m.Artifacts) != 2 || len(m.Content.Items) < 1 || len(m.Content.Items) > 10000 {
		return m, ErrIntegrity
	}
	seen := map[string]bool{}
	for _, a := range m.Artifacts {
		if !digest.MatchString(a.SHA) || a.Size <= 0 || a.Size > MaxBytes || a.Count != len(m.Content.Items) || seen[a.Format] || seen[a.ID] {
			return m, ErrIntegrity
		}
		seen[a.Format] = true
		seen[a.ID] = true
		if (a.Format != "JSON" || a.Name != "records.json") && (a.Format != "JSONL" || a.Name != "records.jsonl") {
			return m, ErrUnsupported
		}
	}
	previous := big.NewInt(0)
	assignments := map[string]bool{}
	for _, i := range m.Content.Items {
		n, ok := new(big.Int).SetString(i.Source.ID, 10)
		if !ok || !decimal.MatchString(i.Source.ID) || n.Cmp(previous) <= 0 || i.Source.Kind != "IMMUTABLE_SUBMISSION" || i.Source.Namespace != c.Namespace.String() || i.Project != c.Project || !decimal.MatchString(i.Task) || !decimal.MatchString(i.Assignment) || !decimal.MatchString(i.Revision) || assignments[i.Assignment] || len(i.Media) != 0 {
			return m, ErrIntegrity
		}
		previous = n
		assignments[i.Assignment] = true
	}
	return m, nil
}
func rawString(m map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(m[k], &s)
	return s
}
func rawID(m map[string]json.RawMessage, k string) string {
	v := bytes.TrimSpace(m[k])
	if len(v) > 0 && v[0] == '"' {
		var s string
		_ = json.Unmarshal(v, &s)
		return s
	}
	return string(v)
}
func rawObject(m map[string]json.RawMessage, k string) map[string]json.RawMessage {
	var o map[string]json.RawMessage
	_ = json.Unmarshal(m[k], &o)
	return o
}
func textFields(config string) ([]string, error) {
	d := xml.NewDecoder(strings.NewReader(config))
	var fields []string
	depth, roots := 0, 0
	allowed := map[string]bool{"View": true, "Text": true, "Choices": true, "Choice": true, "Labels": true, "Label": true, "Header": true}
	for {
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, ErrUnsupported
		}
		if _, ok := t.(xml.Directive); ok {
			return nil, ErrUnsupported
		}
		if _, ok := t.(xml.ProcInst); ok {
			return nil, ErrUnsupported
		}
		if text, ok := t.(xml.CharData); ok && depth == 0 && len(bytes.TrimSpace(text)) != 0 {
			return nil, ErrUnsupported
		}
		if _, ok := t.(xml.EndElement); ok {
			depth--
		}
		if n, ok := t.(xml.StartElement); ok {
			if depth == 0 {
				roots++
				if roots != 1 || n.Name.Local != "View" {
					return nil, ErrUnsupported
				}
			}
			depth++
			if n.Name.Space != "" || !allowed[n.Name.Local] {
				return nil, ErrUnsupported
			}
			if n.Name.Local == "Text" {
				v := ""
				for _, a := range n.Attr {
					if strings.EqualFold(a.Name.Local, "valueType") && a.Value != "text" {
						return nil, ErrUnsupported
					}
					if a.Name.Local == "value" {
						v = a.Value
					}
				}
				if !strings.HasPrefix(v, "$") || len(v) < 2 {
					return nil, ErrUnsupported
				}
				fields = append(fields, v[1:])
			}
		}
	}
	if len(fields) == 0 || depth != 0 || roots != 1 {
		return nil, ErrUnsupported
	}
	return fields, nil
}
func ValidateBundle(m Manifest, blobs map[string][]byte) error {
	if len(blobs) != 3 {
		return ErrIntegrity
	} // exact Manifest plus both native forms
	total := len(blobs["manifest.json"])
	for _, a := range m.Artifacts {
		b, ok := blobs[a.Name]
		total += len(b)
		if !ok || len(b) > MaxBytes || int64(len(b)) != a.Size || Hash(b) != a.SHA {
			return ErrIntegrity
		}
	}
	if total > MaxBytes {
		return ErrUnsupported
	}
	lines := bytes.Split(bytes.TrimSuffix(blobs["records.jsonl"], []byte("\n")), []byte("\n"))
	if len(lines) != len(m.Content.Items) {
		return ErrIntegrity
	}
	var array []any
	if strictJSON(blobs["records.json"], &array) != nil || len(array) != len(lines) {
		return ErrIntegrity
	}
	for k, line := range lines {
		i := m.Content.Items[k]
		var row map[string]json.RawMessage
		var semantic any
		if strictJSON(line, &row) != nil || strictJSON(line, &semantic) != nil || !reflect.DeepEqual(array[k], semantic) || Hash(line) != i.RecordHash {
			return ErrIntegrity
		}
		source := rawObject(row, "source")
		review := rawObject(row, "review")
		snap := rawObject(row, "result_snapshot")
		project := rawObject(snap, "project")
		task := rawObject(snap, "task")
		author := rawObject(row, "submitted_by")
		reviewer := rawObject(review, "reviewer")
		annotation := rawObject(snap, "annotation")
		var cancelled bool
		if !decimal.MatchString(rawID(author, "local_user_id")) || !decimal.MatchString(rawID(reviewer, "local_user_id")) || !decimal.MatchString(rawID(review, "id")) || !decimal.MatchString(rawID(annotation, "id")) || json.Unmarshal(annotation["was_cancelled"], &cancelled) != nil || cancelled {
			return ErrIntegrity
		}
		if _, e := time.Parse(time.RFC3339Nano, rawString(row, "submitted_at")); e != nil {
			return ErrIntegrity
		}
		if _, e := time.Parse(time.RFC3339Nano, rawString(review, "decided_at")); e != nil {
			return ErrIntegrity
		}
		if Hash(row["result_snapshot"]) != i.ResultHash || rawString(row, "result_hash") != i.ResultHash || rawString(source, "namespace") != i.Source.Namespace || rawString(source, "kind") != "submission" || rawID(source, "id") != i.Source.ID || rawID(row, "project_id") != i.Project || rawID(row, "task_id") != i.Task || rawID(row, "assignment_id") != i.Assignment || rawID(row, "revision") != i.Revision || rawID(project, "id") != i.Project || rawID(task, "id") != i.Task || rawString(review, "decision") != "approved" {
			return ErrIntegrity
		}
		config := rawString(project, "label_config")
		if Hash([]byte(config)) != m.Content.XMLHash || m.Content.XMLHash != m.Content.SchemaHash {
			return ErrIntegrity
		}
		fields, e := textFields(config)
		if e != nil {
			return e
		}
		data := rawObject(task, "data")
		for _, f := range fields {
			var value string
			if json.Unmarshal(data[f], &value) != nil {
				return ErrUnsupported
			}
			v := strings.TrimSpace(value)
			if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "/data/") {
				return ErrUnsupported
			}
		}
	}
	return nil
}
func validUUID(v uuid.UUID) bool { return v != uuid.Nil }
