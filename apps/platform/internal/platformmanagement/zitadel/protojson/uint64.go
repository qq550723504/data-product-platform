// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package protojson

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
)

// Uint64 decodes the canonical quoted decimal ProtoJSON representation while
// retaining compatibility with integer-valued JSON emitted by intermediaries.
type Uint64 uint64

func (value *Uint64) UnmarshalJSON(data []byte) error {
	data = bytes.Trim(data, " \t\r\n")
	if len(data) == 0 {
		return errors.New("empty ProtoJSON uint64")
	}

	decimal := string(data)
	if data[0] == '"' {
		if err := json.Unmarshal(data, &decimal); err != nil {
			return err
		}
		if !isUnsignedDecimal(decimal) {
			return errors.New("invalid ProtoJSON uint64")
		}
	} else if !json.Valid(data) || !isCanonicalJSONUnsignedInteger(data) {
		return errors.New("invalid ProtoJSON uint64")
	}
	parsed, err := strconv.ParseUint(decimal, 10, 64)
	if err != nil {
		return err
	}
	*value = Uint64(parsed)
	return nil
}

func isUnsignedDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range []byte(value) {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func isCanonicalJSONUnsignedInteger(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	if value[0] == '0' {
		return len(value) == 1
	}
	if value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, digit := range value[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
