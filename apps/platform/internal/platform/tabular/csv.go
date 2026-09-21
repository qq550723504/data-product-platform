package tabular

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

type Table struct {
	Headers []string
	Rows    []map[string]string
	rawRows []map[string]string
}

func ReadCSV(reader io.Reader) (Table, error) {
	csvReader := csv.NewReader(reader)
	headers, err := csvReader.Read()
	if err != nil {
		return Table{}, fmt.Errorf("read CSV header: %w", err)
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
		if headers[i] == "" {
			return Table{}, fmt.Errorf("CSV header %d is empty", i)
		}
	}
	table := Table{
		Headers: headers,
		Rows:    make([]map[string]string, 0),
		rawRows: make([]map[string]string, 0),
	}
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Table{}, fmt.Errorf("read CSV row: %w", err)
		}
		row := make(map[string]string, len(headers))
		rawRow := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(record) {
				rawRow[header] = record[i]
				row[header] = strings.TrimSpace(record[i])
			}
		}
		table.Rows = append(table.Rows, row)
		table.rawRows = append(table.rawRows, rawRow)
	}
	return table, nil
}

func HeaderSet(headers []string) map[string]struct{} {
	result := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		result[strings.TrimSpace(header)] = struct{}{}
	}
	return result
}

// RawValue returns the original CSV cell value when the table came from
// ReadCSV. Tables assembled directly in tests/adapters fall back to Rows.
// This lets exact-match rules distinguish malformed padding without changing
// the normalized Rows contract used by existing processing code.
func (t Table) RawValue(rowIndex int, field string) string {
	if rowIndex >= 0 && rowIndex < len(t.rawRows) {
		if value, ok := t.rawRows[rowIndex][field]; ok {
			return value
		}
	}
	if rowIndex >= 0 && rowIndex < len(t.Rows) {
		return t.Rows[rowIndex][field]
	}
	return ""
}
