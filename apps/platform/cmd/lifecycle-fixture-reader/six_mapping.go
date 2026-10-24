package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	madmin "github.com/minio/madmin-go/v3"
)

const sixMappingMember = "iam-assets/user_mappings.json"

type sixMapping struct {
	Version int    `json:"version"`
	Policy  string `json:"policy"`
}

func sixMappingJSON(user, name string) []byte {
	raw, _ := json.Marshal(map[string]sixMapping{user: {Version: 1, Policy: name}})
	return raw
}

func sixMappingZIP(user, name string) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.CreateHeader(&zip.FileHeader{Name: sixMappingMember, Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(sixMappingJSON(user, name)); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sixMappingScope(raw []byte, users map[string]string) (string, string, error) {
	denied := errors.New("IAM import must contain one exact current-user policy mapping only")
	if len(raw) == 0 || len(raw) > 2048 {
		return "", "", denied
	}
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(r.File) != 1 || r.Comment != "" {
		return "", "", denied
	}
	f := r.File[0]
	if f.Name != sixMappingMember || !f.Mode().IsRegular() || f.Method != zip.Store || f.Flags&1 != 0 || f.UncompressedSize64 > 1024 || f.Comment != "" {
		return "", "", denied
	}
	stream, err := f.Open()
	if err != nil {
		return "", "", denied
	}
	data, err := io.ReadAll(io.LimitReader(stream, 1025))
	stream.Close()
	if err != nil || len(data) > 1024 {
		return "", "", denied
	}
	// These are our own generated request bytes, not a general policy parser.
	// Canonical equality rejects duplicate keys, whitespace-hidden extra fields,
	// additional users and every other IAM asset without implementing ZIP/JSON.
	for user, name := range users {
		if user != "" && name != "" && bytes.Equal(data, sixMappingJSON(user, name)) {
			canonical, err := sixMappingZIP(user, name)
			if err == nil && bytes.Equal(raw, canonical) {
				return user, name, nil
			}
		}
	}
	return "", "", denied
}

// JSON grammar is owned by encoding/json. This bounded token walk adds only
// duplicate-name rejection, which Decoder.DisallowUnknownFields does not do.
func sixUniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 8 {
			return errors.New("IAM result nesting exceeds fixed contract")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		if delim != '{' && delim != '[' {
			return errors.New("IAM result delimiter invalid")
		}
		seen := map[string]bool{}
		for d.More() {
			if delim == '{' {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return errors.New("IAM result duplicate or invalid member")
				}
				seen[name] = true
			}
			if err := visit(depth + 1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
			return errors.New("IAM result end invalid")
		}
		return nil
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("IAM result has trailing content")
	}
	return nil
}

func sixImportOutcome(raw []byte, user, name string) error {
	denied := errors.New("IAM import result is not one confirmed exact policy association")
	if len(raw) == 0 || len(raw) > 65536 || sixUniqueJSON(raw) != nil {
		return denied
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return denied
	}
	for _, key := range []string{"skipped", "removed", "added", "failed"} {
		value, ok := fields[key]
		if !ok || len(value) == 0 || value[0] != '{' {
			return denied
		}
	}
	var result madmin.ImportIAMResult
	if strictPostV22(raw, &result) != nil {
		return denied
	}
	empty := func(v madmin.IAMEntities) bool {
		return len(v.Policies)+len(v.Users)+len(v.Groups)+len(v.ServiceAccounts)+len(v.UserPolicies)+len(v.GroupPolicies)+len(v.STSPolicies) == 0
	}
	f := result.Failed
	if !empty(result.Skipped) || !empty(result.Removed) || len(f.Policies)+len(f.Users)+len(f.Groups)+len(f.ServiceAccounts)+len(f.UserPolicies)+len(f.GroupPolicies)+len(f.STSPolicies) != 0 {
		return denied
	}
	a := result.Added
	if len(a.UserPolicies) != 1 || len(a.UserPolicies[0]) != 1 || len(a.UserPolicies[0][user]) != 1 || a.UserPolicies[0][user][0] != name {
		return denied
	}
	a.UserPolicies = nil
	if !empty(a) {
		return denied
	}
	return nil
}

func sixRaw(c config, name string, raw []byte) error {
	f, err := os.OpenFile(filepath.Join(c.OutputDirectory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(raw)
	return err
}
