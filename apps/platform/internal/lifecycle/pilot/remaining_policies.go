package pilot

import (
	"encoding/json"
	"errors"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Policy construction is fixture composition; provider enforcement and semantic
// comparison remain the existing MinIO policy SDK. No policy engine lives here.
func RemainingPolicies(group string, present []domain.Copy) (map[string]string, map[string][]byte, error) {
	return remainingPolicies(group, present, []string{"s3:DeleteObject", "s3:DeleteObjectVersion"})
}

func remainingPolicies(group string, present []domain.Copy, deleteActions []string) (map[string]string, map[string][]byte, error) {
	scope, err := RemainingScopeFor(group)
	counts := map[string]int{"business-races": 7, "late-hold": 6, "mixed": 4}
	if err != nil || len(present) != counts[group] {
		return nil, nil, errors.New("six policy present membership differs")
	}
	seen := map[string]bool{}
	var inspectStatements, deleteStatements []any
	inspectStatements = append(inspectStatements, map[string]any{"Effect": "Allow", "Action": []string{"s3:ListBucketVersions"}, "Resource": []string{"arn:aws:s3:::dpp-lifecycle307-online", "arn:aws:s3:::dpp-lifecycle307-quarantine"}})
	for _, cp := range present {
		if cp.Validate() != nil || cp.WorkspaceID != scope.Workspace || cp.ProviderInstanceID != scope.Provider || cp.IncarnationID != scope.Incarnation || cp.Key == "fixture/p1.csv" || seen[cp.ID.String()] {
			return nil, nil, errors.New("six policy copy differs")
		}
		seen[cp.ID.String()] = true
		statement := func(actions []string) map[string]any {
			return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::" + cp.Namespace + "/" + cp.Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": cp.ProviderVersion}}}
		}
		inspectStatements = append(inspectStatements, statement([]string{"s3:GetObjectVersion"}))
		// MinIO's official action contract does not support s3:versionid on
		// protection metadata. The frozen adapter/gateway additionally enforce
		// exact selectors; IAM alone constrains these two actions to the key.
		inspectStatements = append(inspectStatements, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectRetention", "s3:GetObjectLegalHold"}, "Resource": []string{"arn:aws:s3:::" + cp.Namespace + "/" + cp.Key}})
		send := false
		for _, key := range []string{"fixture/p2.csv", "fixture/t1.csv", "fixture/t2.csv", "fixture/h2-a.csv", "fixture/m1-a.csv", "fixture/m1-b.csv"} {
			send = send || cp.Key == key
		}
		if send {
			// The pinned native DeleteObjectHandler authorizes DeleteObject even
			// for versionId requests. Both actions retain the SAME exact-version
			// condition; an unversioned remove is still denied by native IAM.
			deleteStatements = append(deleteStatements, statement(deleteActions))
		}
	}
	if len(deleteStatements) != map[string]int{"business-races": 3, "late-hold": 1, "mixed": 2}[group] {
		return nil, nil, errors.New("six policy delete membership differs")
	}
	users, policies := map[string]string{}, map[string][]byte{}
	for kind, statements := range map[string][]any{"inspect": inspectStatements, "delete": deleteStatements} {
		name := RemainingPolicyName(group, kind)
		users[RemainingIdentity(group, kind)] = name
		policies[name], err = json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
		if err != nil {
			return nil, nil, err
		}
	}
	return users, policies, nil
}

// This one real partial identity was created by the failed, sealed c13
// preparation. It must remain disabled without policy/groups and is never
// selected by current enable or worker paths. This is not a legacy fallback.
func RemainingDormantIdentity(group string) (string, string) {
	if group == "business-races" {
		return "i4e524c04808a6960", "lifecycle307-six-i4e524c04808a6960"
	}
	return "", ""
}

// The failed c13 preparation stopped before these five identities were created.
// Current full preflight proves absence again; it never invents disabled users.
func RemainingUncreatedIdentities(group string) []string {
	s, err := RemainingScopeFor(group)
	if err != nil {
		return nil
	}
	suffix := "4e524c04" + s.Run.String()[:8]
	// C20 stopped before native preparation/provisioning. These six users
	// must remain genuinely absent; they are never counted as disabled users.
	uncreatedC20 := []string{"i5a54e01a" + s.Run.String()[:8], "d5a54e01a" + s.Run.String()[:8]}
	if group == "business-races" {
		return append([]string{"d" + suffix}, uncreatedC20...)
	}
	return append([]string{"i" + suffix, "d" + suffix, "iafc0e66c" + s.Run.String()[:8], "dafc0e66c" + s.Run.String()[:8]}, uncreatedC20...)
}

// These actual c14/c15/c16/c17/c18/c19 users remain disabled with a sole policy and no groups.
// Current workers and enable paths select only the new execution's identities.
func RemainingRetainedIdentities(group string) map[string]string {
	result := map[string]string{}
	if group == "business-races" {
		for _, prefix := range []string{"i", "d"} {
			user := prefix + "afc0e66c808a6960"
			result[user] = "lifecycle307-six-" + user
		}
	}
	if scope, err := RemainingScopeFor(group); err == nil {
		for _, execution := range []string{"9f9c67d7", "aa2605a5", "6f40bca9", "4fd5a7e7", "a5d1e569", "9ba214bd", "0e5cb8f3"} {
			for _, prefix := range []string{"i", "d"} {
				user := prefix + execution + scope.Run.String()[:8]
				result[user] = "lifecycle307-six-" + user
			}
		}
	}
	return result
}

// The unassigned c13 canned policy is still inspected, while its user must
// have no association. Current six identities keep the strict sole-policy rule.
func RemainingAuditPolicies(group string, present []domain.Copy) (map[string]string, map[string][]byte, error) {
	users, policies, err := RemainingPolicies(group, present)
	if err != nil {
		return nil, nil, err
	}
	// These are actual frozen retained policies, not current executable ones.
	// Fixing the new policy must never widen or rewrite disabled historical IAM.
	_, retained, err := remainingPolicies(group, present, []string{"s3:DeleteObjectVersion"})
	if err != nil {
		return nil, nil, err
	}
	for user, name := range RemainingRetainedIdentities(group) {
		users[user] = name
		kind := "delete"
		if user[0] == 'i' {
			kind = "inspect"
		}
		frozen := retained[RemainingPolicyName(group, kind)]
		// The real retained c19/c21 policies already had the exact-version action
		// pair. Audit its actual frozen semantics; never rewrite old IAM.
		if scope, scopeErr := RemainingScopeFor(group); scopeErr == nil {
			for _, execution := range []string{"a5d1e569", "9ba214bd", "0e5cb8f3"} {
				if user == map[string]string{"inspect": "i", "delete": "d"}[kind]+execution+scope.Run.String()[:8] {
					frozen = policies[RemainingPolicyName(group, kind)]
				}
			}
		}
		policies[name] = append([]byte(nil), frozen...)
	}
	if user, name := RemainingDormantIdentity(group); user != "" {
		users[user] = ""
		policies[name] = append([]byte(nil), policies[RemainingPolicyName(group, "inspect")]...)
	}
	return users, policies, nil
}
