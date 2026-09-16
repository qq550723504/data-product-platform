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
	table := Table{Headers: headers, Rows: make([]map[string]string, 0)}
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Table{}, fmt.Errorf("read CSV row: %w", err)
		}
		row := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(record) {
				row[header] = strings.TrimSpace(record[i])
			}
		}
		table.Rows = append(table.Rows, row)
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
