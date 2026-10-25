// File-only aggregation of the frozen native verifier's complete receipt.
// MinIO owns policy semantics. Raw SHA values remain audit facts, not IAM identity.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"

	"github.com/minio/pkg/v3/policy"
)

type receipt struct {
	Group     string            `json:"group"`
	Status    string            `json:"status"`
	Inventory string            `json:"inventory_sha256"`
	Copies    []json.RawMessage `json:"copies"`
	Users     map[string]string `json:"disabled_identities"`
	Policies  map[string]string `json:"policy_sha256"`
	Delete    int               `json:"delete"`
	Mutations int               `json:"mutations"`
	Source    string            `json:"source_config"`
}
type comparison struct {
	Policy             string `json:"policy"`
	BaselineFileSHA256 string `json:"baseline_file_sha256"`
	FreshFileSHA256    string `json:"fresh_file_sha256"`
	OfficialEquals     bool   `json:"official_equals"`
}
type result struct {
	Status     string       `json:"status"`
	Group      string       `json:"group"`
	Copies     int          `json:"copies"`
	Policies   []comparison `json:"policies"`
	ProviderIO int          `json:"provider_io"`
	Delete     int          `json:"delete"`
}

var hash = regexp.MustCompile(`^[0-9a-f]{64}$`)
var name = regexp.MustCompile(`^lifecycle307-[a-z0-9-]+$`)

func digest(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func readReceipt(dir string) (receipt, error) {
	var r receipt
	raw, e := os.ReadFile(filepath.Join(dir, "native-verification.json"))
	if e != nil {
		return r, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(&r); e != nil {
		return r, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return r, errors.New("trailing receipt data")
	}
	// Missing zero-valued safety fields must not masquerade as explicit zero.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if len(fields) != 9 || string(fields["delete"]) != "0" || string(fields["mutations"]) != "0" || r.Status != "PASS_EXACT_NATIVE_READONLY" || !hash.MatchString(r.Inventory) || r.Source != "remaining.json" {
		return r, errors.New("native receipt incomplete or unsafe")
	}
	return r, nil
}
func policyFiles(dir string, policies map[string]string) error {
	files, e := filepath.Glob(filepath.Join(dir, "native-policy-*.json"))
	if e != nil || len(files) != len(policies) {
		return errors.New("policy file membership differs")
	}
	for _, f := range files {
		n := filepath.Base(f)
		n = n[len("native-policy-") : len(n)-len(".json")]
		if _, ok := policies[n]; !ok {
			return errors.New("extra policy file")
		}
	}
	for n, h := range policies {
		if !name.MatchString(n) || !hash.MatchString(h) {
			return errors.New("invalid policy identity or raw digest")
		}
	}
	return nil
}
func check(baseline, fresh, group string) (result, error) {
	out := result{Group: group}
	counts := map[string][2]int{"business-races": {8, 4}, "late-hold": {6, 2}, "mixed": {4, 2}}
	count, ok := counts[group]
	if !ok {
		return out, errors.New("unknown finite group")
	}
	a, e := readReceipt(baseline)
	if e != nil {
		return out, e
	}
	b, e := readReceipt(fresh)
	if e != nil {
		return out, e
	}
	if len(a.Copies) != count[0] || len(b.Copies) != count[0] || a.Inventory != b.Inventory {
		return out, errors.New("inventory membership differs")
	}
	return compareNativeReceipts(baseline, fresh, group, a, b, count[1])
}

func compareNativeReceipts(baseline, fresh, group string, a, b receipt, identities int) (result, error) {
	out := result{Group: group}
	if a.Group != group || b.Group != group || len(a.Copies) != len(b.Copies) || !reflect.DeepEqual(a.Users, b.Users) || len(a.Users) != identities || len(a.Policies) != identities || len(b.Policies) != identities {
		return out, errors.New("inventory or disabled identity binding differs")
	}
	for i := range a.Copies {
		var x, y any
		dx, dy := json.NewDecoder(bytes.NewReader(a.Copies[i])), json.NewDecoder(bytes.NewReader(b.Copies[i]))
		dx.UseNumber()
		dy.UseNumber()
		if dx.Decode(&x) != nil || dy.Decode(&y) != nil || !reflect.DeepEqual(x, y) {
			return out, errors.New("exact version/content/protection differs")
		}
	}
	if e := policyFiles(baseline, a.Policies); e != nil {
		return out, e
	}
	if e := policyFiles(fresh, b.Policies); e != nil {
		return out, e
	}
	for user, n := range a.Users {
		if user == "" {
			return out, errors.New("empty disabled identity")
		}
		if _, ok := a.Policies[n]; !ok {
			return out, errors.New("user policy missing")
		}
	}
	policyNames := make([]string, 0, len(a.Policies))
	for n := range a.Policies {
		policyNames = append(policyNames, n)
	}
	sort.Strings(policyNames)
	for _, n := range policyNames {
		if _, ok := b.Policies[n]; !ok {
			return out, errors.New("policy name substituted")
		}
		x, e := os.ReadFile(filepath.Join(baseline, "native-policy-"+n+".json"))
		if e != nil {
			return out, e
		}
		y, e := os.ReadFile(filepath.Join(fresh, "native-policy-"+n+".json"))
		if e != nil {
			return out, e
		}
		if !json.Valid(x) || !json.Valid(y) {
			return out, errors.New("policy must be exactly one valid JSON value")
		}
		p, e := policy.ParseConfig(bytes.NewReader(x))
		if e != nil {
			return out, e
		}
		q, e := policy.ParseConfig(bytes.NewReader(y))
		if e != nil || !p.Equals(*q) {
			return out, errors.New("official policy semantics differ")
		}
		out.Policies = append(out.Policies, comparison{n, digest(x), digest(y), true})
	}
	out.Status = "PASS_COMPLETE_NATIVE_AGGREGATION"
	out.Copies = len(b.Copies)
	return out, nil
}
func main() {
	baseline := flag.String("baseline", "", "frozen readonly receipt directory")
	fresh := flag.String("fresh", "", "new readonly receipt directory")
	group := flag.String("group", "", "finite group")
	postV22 := flag.Bool("post-v22", false, "explicit post-v22 17-version contract")
	expectation := flag.String("expectation", "", "post-v22 public execution bindings")
	inventory := flag.String("inventory", "", "actual present inventory bytes")
	flag.Parse()
	if *baseline == "" || *fresh == "" || *group == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "exact baseline/fresh/group required")
		os.Exit(1)
	}
	var out result
	var e error
	if *postV22 {
		out, e = checkPostV22(*baseline, *fresh, *group, *expectation, *inventory)
	} else if *expectation != "" || *inventory != "" {
		e = errors.New("post-v22 arguments require explicit mode")
	} else {
		out, e = check(*baseline, *fresh, *group)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "readonly aggregation denied:", e)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(1)
	}
}
