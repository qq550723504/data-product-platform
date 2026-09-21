package hop

import (
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestManagedWorkflowPinsHopDefinitionChecksum(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	pipelinePath := filepath.Join(root, "examples", "enterprise-activity", "hop", "aggregate-energy-monthly.hpl")
	workflowPath := filepath.Join(root, "examples", "enterprise-activity", "workflow", "energy-monthly-hop-v1.yaml")

	pipeline, err := os.ReadFile(pipelinePath)
	if err != nil {
		t.Fatalf("read Hop pipeline: %v", err)
	}
	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read managed workflow: %v", err)
	}

	var workflow struct {
		Spec struct {
			ManagedExecution struct {
				DefinitionRef    string `yaml:"definitionRef"`
				DefinitionSHA256 string `yaml:"definitionSha256"`
			} `yaml:"managedExecution"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(workflowBytes, &workflow); err != nil {
		t.Fatalf("decode managed workflow: %v", err)
	}

	if workflow.Spec.ManagedExecution.DefinitionRef != "examples/enterprise-activity/hop/aggregate-energy-monthly.hpl" {
		t.Fatalf("unexpected definitionRef %q", workflow.Spec.ManagedExecution.DefinitionRef)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(canonicalDefinitionBytes(pipeline)))
	if workflow.Spec.ManagedExecution.DefinitionSHA256 != checksum {
		t.Fatalf("workflow pins checksum %s, pipeline checksum is %s", workflow.Spec.ManagedExecution.DefinitionSHA256, checksum)
	}
}

func TestWrapPipelineConfiguration(t *testing.T) {
	raw := []byte("<?xml version=\"1.0\"?><pipeline><info><name>demo</name></info></pipeline>")
	wrapped := string(wrapPipelineConfiguration(raw))
	if !strings.HasPrefix(wrapped, "<pipeline_configuration><pipeline>") {
		t.Fatalf("unexpected wrapper: %s", wrapped)
	}
	if !strings.Contains(wrapped, "<pipeline_execution_configuration>") {
		t.Fatalf("execution configuration missing: %s", wrapped)
	}
}

func TestReferenceEnergyAggregateFixture(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	inputPath := filepath.Join(root, "examples", "enterprise-activity", "hop", "energy-standardized-input.csv")
	expectedPath := filepath.Join(root, "examples", "enterprise-activity", "hop", "energy-monthly-expected.csv")

	input, err := readCSV(inputPath)
	if err != nil {
		t.Fatalf("read input fixture: %v", err)
	}
	expected, err := readCSV(expectedPath)
	if err != nil {
		t.Fatalf("read expected fixture: %v", err)
	}

	if len(input) < 2 || len(expected) < 2 {
		t.Fatal("fixtures must contain header and data rows")
	}
	aggregated := map[string]float64{}
	for _, row := range input[1:] {
		if len(row) != 3 {
			t.Fatalf("unexpected input row: %#v", row)
		}
		value, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			t.Fatalf("parse energy value %q: %v", row[2], err)
		}
		aggregated[row[0]+"\x00"+row[1]] += value
	}

	actual := make([]string, 0, len(aggregated))
	for key, value := range aggregated {
		parts := strings.Split(key, "\x00")
		actual = append(actual, fmt.Sprintf("%s,%s,%g", parts[0], parts[1], value))
	}
	sort.Strings(actual)

	want := make([]string, 0, len(expected)-1)
	for _, row := range expected[1:] {
		want = append(want, strings.Join(row, ","))
	}
	sort.Strings(want)
	if strings.Join(actual, "\n") != strings.Join(want, "\n") {
		t.Fatalf("aggregate mismatch\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(actual, "\n"))
	}
}

func TestParameterName(t *testing.T) {
	if got := parameterName(" energy-standardized "); got != "ENERGY_STANDARDIZED" {
		t.Fatalf("expected ENERGY_STANDARDIZED, got %q", got)
	}
}

func readCSV(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return csv.NewReader(file).ReadAll()
}
