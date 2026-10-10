package labelstudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// PythonSnapshotJSON implements the frozen json.dumps(...,sort_keys=True,
// separators=(',',':'),ensure_ascii=False) contract, including Python float
// spelling. No provider label_config_hash integer is used.
func PythonSnapshotJSON(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("invalid UTF-8 snapshot")
	}
	if err := validateSnapshotUnicode(raw); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := readPythonJSON(d)
	if err != nil {
		return nil, err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("trailing snapshot content")
	}
	var b bytes.Buffer
	if err := writePythonJSON(&b, value); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func writePythonJSON(b *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		writePythonString(b, x)
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			if s == "-0" {
				s = "0"
			}
			b.WriteString(s)
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("non-finite snapshot number")
		}
		scientific := strconv.FormatFloat(f, 'e', -1, 64)
		parts := strings.Split(scientific, "e")
		exponent, _ := strconv.Atoi(parts[1])
		if exponent >= -4 && exponent < 16 {
			fixed := strconv.FormatFloat(f, 'f', -1, 64)
			if !strings.Contains(fixed, ".") {
				fixed += ".0"
			}
			b.WriteString(fixed)
		} else {
			b.WriteString(parts[0])
			b.WriteByte('e')
			if exponent < 0 {
				b.WriteByte('-')
				exponent = -exponent
			} else {
				b.WriteByte('+')
			}
			b.WriteString(fmt.Sprintf("%02d", exponent))
		}
	case []any:
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writePythonJSON(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writePythonString(b, k)
			b.WriteByte(':')
			if err := writePythonJSON(b, x[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("unsupported snapshot JSON")
	}
	return nil
}
func writePythonString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteByte('\\')
			b.WriteByte('"')
		case '\\':
			b.WriteString("\\\\")
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 32 {
				fmt.Fprintf(b, "\\u%04x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

func readPythonJSON(d *json.Decoder) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		m := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			s, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, exists := m[s]; exists {
				return nil, fmt.Errorf("duplicate snapshot key")
			}
			v, err := readPythonJSON(d)
			if err != nil {
				return nil, err
			}
			m[s] = v
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, fmt.Errorf("invalid object")
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			v, err := readPythonJSON(d)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, fmt.Errorf("invalid array")
		}
		return a, nil
	default:
		return nil, fmt.Errorf("unexpected delimiter")
	}
}
func validateSnapshotUnicode(raw []byte) error {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return fmt.Errorf("invalid string escape")
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return fmt.Errorf("invalid unicode escape")
		}
		n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return err
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return fmt.Errorf("unpaired low surrogate")
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return fmt.Errorf("unpaired high surrogate")
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return fmt.Errorf("invalid surrogate pair")
			}
			i += 6
		}
	}
	return nil
}
