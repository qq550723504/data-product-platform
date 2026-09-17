package csvinput

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestCSVWithOptionalBOM(t *testing.T) {
	for _, prefix := range []string{"", "\ufeff"} {
		for _, header := range []string{"source_company_id,company_name", "\"source_company_id\",\"company_name\""} {
			t.Run(prefix+header, func(t *testing.T) {
				reader := NewReader(strings.NewReader(prefix + header + "\r\nA,\"测试,主体\"\r\n"))
				actual, err := reader.ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				expected := [][]string{{"source_company_id", "company_name"}, {"A", "测试,主体"}}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("rows = %#v", actual)
				}
			})
		}
	}
}
func TestCSVEmptyAndBOMOnly(t *testing.T) {
	for _, value := range []string{"", "\ufeff"} {
		if _, err := NewReader(strings.NewReader(value)).Read(); err != io.EOF {
			t.Fatalf("want EOF, got %v", err)
		}
	}
}
func TestOnlyLeadingBOMIsSkipped(t *testing.T) {
	rows, err := NewReader(strings.NewReader("id,name\na,\ufeffinside")).ReadAll()
	if err != nil || rows[1][1] != "\ufeffinside" {
		t.Fatalf("content changed: %#v %v", rows, err)
	}
}
