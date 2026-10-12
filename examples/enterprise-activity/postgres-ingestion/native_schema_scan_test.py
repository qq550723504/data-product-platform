"""Native SDK contract regressions; no provider success is fabricated as acceptance."""
import contextlib, io, json, signal, tempfile, unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch
import yaml
from metadata.generated.schema.api.data.createTable import CreateTableRequest
from metadata.workflow.metadata import MetadataWorkflow
from metadata.workflow.workflow_output_handler import WorkflowResultStatus
import native_schema_scan as adapter

TARGET = "source.database.schema.energy"

class NativeSchemaContract(unittest.TestCase):
    def invoke(self, records, failures=(), catalog_type="TEXT"):
        cfg = {"dppTableFqn":TARGET,"source":{"type":"postgres","serviceName":"source",
            "serviceConnection":{"config":{"type":"Postgres","username":"reader","hostPort":"localhost:5432","database":"database"}},
            "sourceConfig":{"config":{"type":"DatabaseMetadata"}}},
            "workflowConfig":{"openMetadataServerConfig":{"hostPort":"http://localhost/api","securityConfig":{"jwtToken":"private-test-secret"}}}}
        source = SimpleNamespace(run=lambda: iter(records))
        workflow = SimpleNamespace(source=source, execute=lambda:list(source.run()), raise_from_status=lambda:None,
            result_status=lambda:WorkflowResultStatus.SUCCESS, workflow_steps=lambda:[SimpleNamespace(get_status=lambda:SimpleNamespace(failures=failures))],stop=lambda:None)
        cached = {"id":"cached-table","fullyQualifiedName":TARGET,
            "columns":[{"name":"energy_kwh","dataType":catalog_type,"constraint":"NOT_NULL"}]}
        response = io.BytesIO(json.dumps(cached).encode())
        output = io.StringIO()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)/"native.yaml";path.write_text(yaml.safe_dump(cfg))
            with patch.object(MetadataWorkflow,"create",return_value=workflow), patch("sys.stdin",io.StringIO("RUN\n")), \
                 contextlib.redirect_stdout(output), patch("urllib.request.urlopen",return_value=response) as read:
                self.catalog_read = read
                try: adapter.run(path)
                finally: signal.alarm(0)
        return output.getvalue(), read

    def table(self, name="energy"):
        return CreateTableRequest(name=name,databaseSchema="source.database.schema",
            columns=[{"name":"energy_kwh","dataType":"TEXT","constraint":"NOT_NULL"}])

    def test_empty_or_foreign_discovery_cannot_use_cached_table(self):
        for records in ([],[self.table("other")]):
            with self.subTest(records=records):
                with self.assertRaisesRegex(RuntimeError,"did not emit the exact"):
                    self.invoke(records)
                self.catalog_read.assert_not_called()

    def test_sink_failure_cannot_use_source_success(self):
        with self.assertRaisesRegex(RuntimeError,"failed step"):
            self.invoke([self.table()],failures=["sink rejected"])

    def test_fresh_source_and_cached_catalog_must_agree(self):
        with self.assertRaisesRegex(RuntimeError,"differs from this native"):
            self.invoke([self.table()],catalog_type="INT")

    def test_exact_native_record_passes_through_and_produces_columns(self):
        output,_ = self.invoke([self.table()])
        result=json.loads(output.splitlines()[-1])
        self.assertEqual(result["columns"],[{"name":"energy_kwh","type":"TEXT","constraint":"NOT_NULL"}])

if __name__=="__main__": unittest.main()
