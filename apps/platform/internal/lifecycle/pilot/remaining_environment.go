package pilot

import "errors"

// Finite retained fixture identities, shared by strict source/restore readers
// and file-only aggregation. These are not platform runtime configuration.
func RemainingEnvironmentSources() map[string][4]string {
	return map[string][4]string{
		"business-races": {"9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0", "f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce", "6354fa508f7adb9ac66fc895e539786a5575e0b3754d9fdb2677d0557ab7c942", "6330308dafa1d377ed8dda5a4f869cd5d5df21583fa334f3493e739a112e8001"},
		"late-hold":      {"982c4e8d2e13331ee94f62148f1bfa6846418a743599cce2270748f8205ed0db", "f26f011ae3d6e83d012fd914462ee00c153a077266d38e37a57856b2e5148061", "e27a29ee55732447bff932d15bcb83339784089c00f03b9139232417fe1a8847", "76172df616268153354c919a89701e42312957b3f9b89b26cdeaa082412c4130"},
		"mixed":          {"cb50e06b7cd575e7da04b5ff64498ec61a430607f270d4bf3102c1372dca2a82", "66c17878149ff1959339bc60e319f3b0494786e62da33b77017bfc8aab13613d", "77f72e9ef0781527a7e8e5ce4a6cd8d36018e527a9ea853ae72d5f8fff9ea0eb", "37fdff1b2663585acd91909f679835771f3225c342d04cc97991ba2848048d8b"},
	}
}

type RemainingEnvironmentMap struct {
	Execution, Profile string
	Groups             []struct {
		Group           string
		Source, Restore [3]string
	}
}

func (m RemainingEnvironmentMap) Validate() error {
	if m.Execution != RemainingExecution || m.Profile != RemainingProfile || len(m.Groups) != 3 {
		return errors.New("six environment map authority differs")
	}
	seen, ids := map[string]bool{}, map[string]bool{}
	for _, g := range m.Groups {
		source, ok := RemainingEnvironmentSources()[g.Group]
		if !ok || seen[g.Group] || g.Source != [3]string{source[0], source[1], source[2]} {
			return errors.New("six source map differs")
		}
		seen[g.Group] = true
		for _, role := range [][3]string{g.Source, g.Restore} {
			for _, id := range role {
				if len(id) != 64 || ids[id] {
					return errors.New("six environment role aliases")
				}
				for _, c := range id {
					if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
						return errors.New("six environment CID is not canonical")
					}
				}
				ids[id] = true
			}
		}
	}
	return nil
}

func (m RemainingEnvironmentMap) Matches(group, role string, actual [3]string) bool {
	if m.Validate() != nil || (role != "SOURCE" && role != "RESTORE") {
		return false
	}
	for _, g := range m.Groups {
		if g.Group == group {
			if role == "SOURCE" {
				return actual == g.Source
			}
			return actual == g.Restore
		}
	}
	return false
}
