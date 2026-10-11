package domain

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDistinctSourceResources(t *testing.T) {
	r := Request{WorkspaceID: uuid.New(), ActorID: uuid.New(), PrincipalRef: "operator", RequestKey: "request", BatchKey: "batch", ConnectionRef: "connection", InstanceRef: "instance", ConfigHash: strings.Repeat("a", 64), TemplateHash: strings.Repeat("b", 64), SchemaHash: strings.Repeat("c", 64)}
	for _, code := range []string{"enterprise_raw", "lease_raw", "energy_raw"} {
		r.Slots = append(r.Slots, Slot{Code: code, DatasetID: uuid.New(), ResourceID: uuid.New(), Columns: []string{"id"}, Rows: 1})
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Slots[1].ResourceID = r.Slots[0].ResourceID
	if _, err := r.Fingerprint(); err != ErrManifest {
		t.Fatal("distinct datasets sharing one source resource were accepted")
	}
}

func TestExactTextCSV(t *testing.T) {
	original := []byte("id,value\n\"1\",\"  中文,\"\"引号\"\"\n下一行  \"\n2,\"\"\n3,2020/06/08\n4,-999\n")
	out := Output{Bytes: int64(len(original)), SHA256: Hash(original), Rows: 4, Columns: []string{"id", "value"}}
	if err := ValidateContent(out, original); err != nil {
		t.Fatal("original strings were rejected")
	}
	cases := map[string][]byte{"BOM": append([]byte{0xef, 0xbb, 0xbf}, original...), "CRLF": bytes.ReplaceAll(original, []byte("\n"), []byte("\r\n")), "invalid-UTF8": append(original, 255)}
	cases["NUL"] = append(original, 0)
	cases["no-terminal-LF"] = original[:len(original)-1]
	cases["wrong-header"] = bytes.Replace(original, []byte("id,value"), []byte("value,id"), 1)
	cases["wrong-width"] = []byte("id,value\n1,a,b\n")
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			changed := out
			changed.Bytes = int64(len(data))
			changed.SHA256 = Hash(data)
			if ValidateContent(changed, data) == nil {
				t.Fatal("invalid CSV accepted with a matching byte hash")
			}
		})
	}
	changed := out
	changed.Rows = 3
	if ValidateContent(changed, original) == nil {
		t.Fatal("row count mismatch accepted")
	}
	changed = out
	changed.SHA256 = Hash(bytes.TrimSpace(original))
	if ValidateContent(changed, original) == nil {
		t.Fatal("a normalized checksum accepted for original bytes")
	}
}
