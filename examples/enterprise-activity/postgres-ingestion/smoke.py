#!/usr/bin/env python3
"""B1 only: real Docker PostgreSQL/Hop, synthetic data, no Core admission.

Uses stdlib + Docker CLI. Fault proxy forwards to the real Hop Server before
dropping responses; it never fabricates provider success. Artifacts exclude
secrets, raw source samples and HTTP Authorization headers.
"""
import argparse
import base64
import csv
import gzip
import hashlib
import http.client
import http.server
import io
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import xml.etree.ElementTree as ET

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[2]
SLOTS = {
    "enterprise": ("source_company_id company_name unified_social_credit_code legal_representative registered_address entry_date company_status contact_name mobile email".split(), [0], 6),
    "lease": ("lease_id source_company_id company_name contract_start contract_end rent_amount payment_due_date payment_date lease_status".split(), [0], 10),
    "energy": ("meter_id source_company_id company_name reading_time energy_kwh".split(), [0, 3, 1], 14),
}
HOP_IMAGE = "apache/hop:2.19.0@sha256:5f285c86523439ab3859f1b75e1fab1b4d7d958daa0349e609ef45be403a854d"
PG_IMAGE = "postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea"
JDBC_URL = "https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.14/postgresql-42.7.14.jar"
JDBC_SHA256 = "73914527305a40cce504b0d3d90b23caf565136912d607ae8ae7c5895512332c"


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def source_sha(path):
    # Git text sources have LF in the index; Windows checkouts may use CRLF.
    # Artifact SHA-256 below always hashes the actual CSV bytes without this.
    return sha(path.read_bytes().replace(b"\r\n", b"\n"))


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def command(args, data=None, check=True, timeout=120):
    result = subprocess.run(args, input=data, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout)
    if check and result.returncode:
        # Command output can contain driver diagnostics or bootstrap secrets.
        raise RuntimeError(f"{args[0]} command failed (exit {result.returncode}); raw output withheld")
    return result


def fixture(table):
    path = HERE.parent / "data" / f"{table}.csv"
    with path.open(encoding="utf-8-sig", newline="") as handle:
        rows = list(csv.reader(handle, strict=True))
    columns, keys, count = SLOTS[table]
    require(rows[0] == columns and len(rows) - 1 == count, f"{table} frozen fixture mismatch")
    require(all(len(row) == len(columns) for row in rows[1:]), "fixture row width")
    return sorted(rows[1:], key=lambda row: tuple(row[k] for k in keys))


class FaultProxy(http.server.ThreadingHTTPServer):
    """Bounded test transport with a real upstream, recording only safe wire data."""
    daemon_threads = True

    def __init__(self, upstream, authorization, port=0, content_type="text/xml;charset=UTF-8"):
        super().__init__(("127.0.0.1", port), ProxyHandler)
        self.upstream = upstream
        self.authorization = authorization
        self.content_type = content_type
        self.drop = None
        self.unavailable = False
        self.requests = []
        self.responses = []
        self.observed_ids = []  # Test oracle only; recovery never consumes these.
        self.before_start = None  # B2 fault hook; unused in the B1 contract run.
        self.strip_status_identity = False


class ProxyHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.forward()

    def do_POST(self):
        self.forward()

    def forward(self):
        proxy = self.server
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        proxy.requests.append({"method": self.command, "url": self.path,
                               "body": body.decode("utf-8")})
        if proxy.unavailable:
            self.send_error(503)
            return
        if proxy.before_start and "/startPipeline" in self.path:
            hook, proxy.before_start = proxy.before_start, None
            hook()
        req = urllib.request.Request(proxy.upstream + self.path, data=body if body else None,
                                     method=self.command, headers={
                                         "Authorization": proxy.authorization,
                                         "Content-Type": proxy.content_type})
        try:
            with urllib.request.urlopen(req, timeout=25) as response:
                status, content = response.status, response.read()
        except urllib.error.HTTPError as error:
            status, content = error.code, error.read()
        proxy.responses.append(content)
        if proxy.strip_status_identity and "/pipelineStatus" in self.path and status == 200:
            value = json.loads(content)
            value.pop("id", None)
            value.pop("pipelineName", None)
            content = canonical(value)  # Corrupt an actual reply; never invent success.
        if "/registerPipeline" in self.path and status == 200:
            try:
                proxy.observed_ids.append(ET.fromstring(content).findtext("id"))
            except ET.ParseError:
                pass
        if proxy.drop and proxy.drop in self.path:
            proxy.drop = None
            self.close_connection = True
            self.connection.shutdown(socket.SHUT_RDWR)
            self.connection.close()
            return
        self.send_response(status)
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)


class Trial:
    def __init__(self, output, retain=False, durable_start=None):
        self.output = output
        self.retain = retain
        self.durable_start = durable_start
        self.volumes = []
        if retain:
            from retained_runtime import private_directory
            self.private = private_directory("dpp-pgi-private-")
        else:self.private = Path(tempfile.mkdtemp(prefix="dpp-pgi-private-"))
        self.runtime = self.private / "runtime"
        self.runtime.mkdir()
        self.staging = self.output / "staging"
        self.staging.mkdir()
        self.staging.chmod(0o777)
        self.run_id = str(uuid.uuid4())
        self.identity = "dpp-pgi-" + uuid.UUID(self.run_id).hex[:12]
        self.pg = self.identity + "-source"
        self.hop = self.identity + "-hop"
        self.network = self.identity + "-net"
        self.passwords = [secrets.token_hex(24) for _ in range(3)]
        self.proxy = None
        self.report = {"contract": "postgres-ingestion-v1/B1", "base_sha":
                       command(["git", "-C", str(REPO), "rev-parse", "HEAD"]).stdout.decode().strip(),
                       "scope": "real engines / synthetic source / local staging only",
                       "checks": {}, "attempts": [], "core_admission": "NOT_RUN",
                       "object_storage": "NOT_RUN", "browser_acceptance": "NOT_RUN"}
        self.report["implementation_sha256"] = {
            str(file.relative_to(REPO)).replace("\\", "/"): source_sha(file)
            for file in [HERE / ".gitattributes", HERE / "smoke.py", HERE / "start-hop.sh", HERE / "Dockerfile.hop",
                         HERE / "source.sql", HERE / "extract.hpl",
                         HERE / "metadata/rdbms/dpp-source.json",
                         HERE / "metadata/pipeline-run-configuration/local.json",
                         REPO / ".github/workflows/ci.yml"]
        }

    def record(self, name, detail):
        self.report["checks"][name] = {"status": "PASS", "detail": detail}
        self.save()
        print(f"PASS {name}", flush=True)

    def docker(self, *args, **kwargs):
        return command(["docker", *args], **kwargs)

    def psql(self, sql, reader=False, check=True):
        args = ["exec", "-i", self.pg, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1",
                "-U", "dpp_reader" if reader else "postgres", "-d", "dpp_ingestion_source"]
        if reader:
            # Actual reader login, not SET ROLE from a superuser session (whose
            # session_user could switch to an owner despite current_role).
            args = ["exec", "-i", self.pg, "sh", "-c",
                    'export PGPASSWORD="$(cat /run/secrets/reader-password)"; exec psql -h 127.0.0.1 -X -qAt -v ON_ERROR_STOP=1 -U dpp_reader -d dpp_ingestion_source']
        return self.docker(*args, data=sql.encode(), check=check)

    def wait_pg(self):
        for _ in range(60):
            if self.docker("exec", self.pg, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", check=False).returncode == 0:
                return
            time.sleep(1)
        raise RuntimeError("source readiness timeout")

    def setup(self):
        for image in (PG_IMAGE, HOP_IMAGE):
            self.docker("image", "inspect", image, check=False)
            self.docker("pull", image, timeout=600)
        versions = {}
        for label, image in (("postgres", PG_IMAGE), ("hop", HOP_IMAGE)):
            meta = json.loads(self.docker("image", "inspect", image).stdout)[0]
            versions[label] = {"tag": image, "id": meta["Id"], "digests": meta["RepoDigests"]}
        self.report["runtime"] = versions
        build = self.private / "build"
        build.mkdir()
        with urllib.request.urlopen(JDBC_URL, timeout=30) as response:
            driver = response.read()
        require(sha(driver) == JDBC_SHA256, "driver checksum mismatch")
        (build / "postgresql-42.7.14.jar").write_bytes(driver)
        self.engine_image = "dpp-pgi-hop-jdbc:42.7.14"
        self.docker("build", "-q", "-t", self.engine_image, "-f", str(HERE / "Dockerfile.hop"), str(build), timeout=180)
        self.report["runtime"]["jdbc"] = {"version": "42.7.14", "url": JDBC_URL, "sha256": JDBC_SHA256,
            "engine_image_id": json.loads(self.docker("image", "inspect", self.engine_image).stdout)[0]["Id"]}
        (self.private / "admin-password").write_text(self.passwords[0])
        (self.runtime / "source-password").write_text(self.passwords[1])
        server = ET.Element("hop-server-config")
        hs = ET.SubElement(server, "hop-server")
        for key, value in (("name", "b1"), ("hostname", "0.0.0.0"), ("port", "8080"),
                           ("username", "b1"), ("password", self.passwords[2])):
            ET.SubElement(hs, key).text = value
        ET.SubElement(server, "metadata_folder").text = "/metadata"
        ET.ElementTree(server).write(self.runtime / "hop-server.xml", encoding="UTF-8")
        for file in self.runtime.iterdir():
            file.chmod(0o644)  # ephemeral, private parent; Hop container UID differs
        # Separate bridge from all existing apps; only Hop's control port is
        # loopback-published. Docker internal bridges suppress port publishing.
        self.docker("network", "create", self.network)
        self.source_host = "source"
        source_bind = "127.0.0.1"
        if getattr(self, "source_port", None):
            if os.name == "nt":
                # Docker Desktop resolves this same endpoint from host and Hop.
                self.source_host = "host.docker.internal"
            else:
                # Bind only the isolated Docker bridge, never a public interface.
                network = json.loads(self.docker("network", "inspect", self.network).stdout)[0]
                self.source_host = source_bind = network["IPAM"]["Config"][0]["Gateway"]
        durable_args = []
        if self.retain:
            volume = self.pg + "-data"
            self.docker("volume", "create", "--label", "dpp.trial.run="+self.run_id, volume)
            self.volumes.append(volume)
            durable_args = ["--label", "dpp.trial.run="+self.run_id, "--mount", f"type=volume,src={volume},dst=/var/lib/postgresql/data"]
            require(self.durable_start is not None, "retained source requires durable storage preflight")
        self.docker("create" if self.retain else "run", *([] if self.retain else ["-d"]), "--name", self.pg, "--network", self.network,
                    *(["-p", f"{source_bind}:{self.source_port}:5432"] if getattr(self, "source_port", None) else []),
                    "--network-alias", "source", "-e", "POSTGRES_DB=dpp_ingestion_source",
                    "-e", "POSTGRES_PASSWORD_FILE=/run/secrets/admin-password",
                    "--mount", f"type=bind,src={self.private / 'admin-password'},dst=/run/secrets/admin-password,readonly",
                    "--mount", f"type=bind,src={self.runtime / 'source-password'},dst=/run/secrets/reader-password,readonly",
                    *durable_args, PG_IMAGE)
        if self.retain:self.durable_start(self.pg, "/var/lib/postgresql/data", volume, "source", self.run_id)
        self.wait_pg()
        # Only temporary synthetic credentials are created. Avoid echo/logging.
        self.psql(f"CREATE ROLE dpp_reader LOGIN NOINHERIT PASSWORD '{self.passwords[1]}';\n" +
                  (HERE / "source.sql").read_text())
        self.load_fixtures()
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            control_port = listener.getsockname()[1]
        hop_args = []
        if self.retain:
            for path in ("files", "tmp"):
                volume = self.hop + "-" + path
                self.docker("volume", "create", "--label", "dpp.trial.run="+self.run_id, volume)
                self.volumes.append(volume)
                hop_args += ["--mount", f"type=volume,src={volume},dst=/{path}"]
        else:hop_args = ["--tmpfs", "/tmp:rw"]
        self.docker("run", "-d", "--name", self.hop, "--network", self.network,
                    *(["--label", "dpp.trial.run="+self.run_id] if self.retain else []),
                    "-p", f"127.0.0.1:{control_port}:8080", "--mount", f"type=bind,src={self.runtime},dst=/run/secrets,readonly",
                    "--mount", f"type=bind,src={HERE / 'metadata'},dst=/metadata,readonly",
                    "--mount", f"type=bind,src={HERE / 'start-hop.sh'},dst=/start-hop.sh,readonly",
                    "--mount", f"type=bind,src={self.staging},dst=/staging",
                    *hop_args, "--entrypoint", "/bin/sh", self.engine_image, "/start-hop.sh")
        for _ in range(15):
            bindings = json.loads(self.docker("inspect", self.hop).stdout)[0]["NetworkSettings"]["Ports"].get("8080/tcp")
            if bindings:
                break
            time.sleep(1)
        require(bindings, "Hop has no published loopback port")
        port = bindings[0]["HostPort"]
        self.upstream = f"http://127.0.0.1:{port}"
        self.authorization = "Basic " + base64.b64encode(("b1:" + self.passwords[2]).encode()).decode()
        self.wait_hop()
        self.proxy = FaultProxy(self.upstream, self.authorization)
        threading.Thread(target=self.proxy.serve_forever, daemon=True).start()
        self.url = "http://127.0.0.1:" + str(self.proxy.server_port)
        self.inventory()

    def load_fixtures(self):
        sql = []
        for table, (columns, _, _) in SLOTS.items():
            sql.append(f"TRUNCATE dpp_source.{table};")
            for row in fixture(table):
                values = ",".join("'" + value.replace("'", "''") + "'" for value in row)
                sql.append(f"INSERT INTO dpp_source.{table} ({','.join(columns)}) VALUES ({values});")
        self.psql("\n".join(sql))

    def wait_hop(self):
        for _ in range(90):
            try:
                req = urllib.request.Request(self.upstream + "/hop/status?xml=Y",
                                             headers={"Authorization": self.authorization})
                with urllib.request.urlopen(req, timeout=2) as response:
                    if response.status == 200:
                        return
            except (OSError, urllib.error.URLError, http.client.HTTPException):
                pass
            time.sleep(1)
        raise RuntimeError("Hop readiness timeout; raw logs withheld")

    def inventory(self):
        details = self.docker("exec", self.hop, "sh", "-c",
                              "java -version 2>&1; find /opt/hop/lib -name 'postgresql-*.jar' -exec sha256sum {} \\;").stdout.decode()
        jars = [line for line in details.splitlines() if ".jar" in line]
        require(len(jars) == 1, "classpath must contain exactly one PostgreSQL driver")
        require(JDBC_SHA256 in jars[0] and '42.7.14.jar' in jars[0], "wrong effective JDBC driver")
        self.report["runtime"]["java_jdbc"] = details.strip().splitlines()
        self.report["runtime"]["postgres_version"] = self.psql("SELECT version();").stdout.decode().strip()
        self.report["template_sha256"] = sha((HERE / "extract.hpl").read_bytes())
        self.report["fixture_sha256"] = {table: sha((HERE.parent / "data" / (table + ".csv")).read_bytes()) for table in SLOTS}

    def request(self, path, params, body=None):
        target = self.url + "/hop/" + path + "?" + urllib.parse.urlencode(params)
        req = urllib.request.Request(target, data=body, headers={"Authorization": self.authorization,
                                                               "Content-Type": "text/xml;charset=UTF-8"})
        with urllib.request.urlopen(req, timeout=30) as response:
            return response.read()

    def preflight(self):
        expected = []
        for table, (columns, _, _) in sorted(SLOTS.items()):
            expected.extend([[table, col, "text", "NO"] for col in columns])
        result = self.psql("COPY (SELECT table_name,column_name,data_type,is_nullable FROM information_schema.columns "
                           "WHERE table_schema='dpp_source' ORDER BY table_name,ordinal_position) TO STDOUT WITH CSV;",
                           reader=True)
        actual = list(csv.reader(io.StringIO(result.stdout.decode())))
        require(actual == expected, "SOURCE_SCHEMA_REJECTED")
        for table in SLOTS:
            columns = SLOTS[table][0]
            nulls = self.psql(f"SELECT count(*) FROM dpp_source.{table} WHERE " +
                               " OR ".join(f"{col} IS NULL" for col in columns) + ";", reader=True)
            require(nulls.stdout.decode().strip() == "0", "SOURCE_NULL_REJECTED")
        return sha(canonical(expected))

    def register(self, attempt, slow=False):
        self.preflight()
        prefix = self.staging / attempt
        prefix.mkdir()
        prefix.chmod(0o777)
        name = self.identity + "-" + attempt
        config = ET.Element("pipeline_configuration")
        pipeline = ET.parse(HERE / "extract.hpl").getroot()
        pipeline.find("info/name").text = name
        if slow:
            # Test-only delays create a measurable in-flight JDBC extraction.
            # This derivative definition gets its own hash in the start fact.
            for hop in pipeline.findall("order/hop"):
                target = hop.findtext("to")
                delay_name = "Delay " + target
                hop.find("to").text = delay_name
                new_hop = ET.SubElement(pipeline.find("order"), "hop")
                for key, value in (("from", delay_name), ("to", target), ("enabled", "Y")):
                    ET.SubElement(new_hop, key).text = value
                transform = ET.SubElement(pipeline, "transform")
                for key, value in (("name", delay_name), ("type", "Delay"), ("copies", "1"),
                                   ("scaletime", "seconds"), ("timeout", "3")):
                    ET.SubElement(transform, key).text = value
        config.append(pipeline)
        execution = ET.SubElement(config, "pipeline_execution_configuration")
        parameter = ET.SubElement(ET.SubElement(execution, "parameters"), "parameter")
        ET.SubElement(parameter, "name").text = "OUTPUT_PREFIX"
        ET.SubElement(parameter, "value").text = "/staging/" + attempt
        for key, value in (("run_configuration", "local"), ("log_level", "Basic"),
                           ("clear_log", "Y"), ("log_file", "N")):
            ET.SubElement(execution, key).text = value
        # The real 2.19.0 API requires a gzip/base64 JSON metadata object.
        # An absent/empty metastore_json is not an empty metadata provider.
        metadata = {
            "rdbms": [json.loads((HERE / "metadata/rdbms/dpp-source.json").read_text())],
            "pipeline-run-configuration": [json.loads((HERE / "metadata/pipeline-run-configuration/local.json").read_text())],
        }
        ET.SubElement(config, "metastore_json").text = base64.b64encode(gzip.compress(canonical(metadata), mtime=0)).decode()
        entry_definition_sha = sha(ET.tostring(config))
        entry = {"attempt": attempt, "name": name, "request_key": self.identity + "/" + attempt,
                 "definition_sha256": entry_definition_sha,
                 "observations": [{"state": "START_FACT", "at": time.time()}]}
        self.report["attempts"].append(entry)
        self.save()  # Test evidence only; B2 still requires Core durable operation.
        result = ET.fromstring(self.request("registerPipeline", {"xml": "Y"}, ET.tostring(config)))
        require(result.findtext("result") == "OK" and result.findtext("id"), "registration rejected")
        entry["remote_id"] = result.findtext("id")
        self.save()
        return entry

    def start(self, entry, accepted=True):
        entry["observations"].append({"state": "START_REQUEST", "at": time.time()})
        self.save()
        response = ET.fromstring(self.request("startPipeline", {"xml": "Y", "name": entry["name"],
                                       "id": entry["remote_id"]}))
        require(response.findtext("result") == ("OK" if accepted else "ERROR"), "unexpected start response")
        entry["observations"].append({"state": "START_ACCEPTED" if accepted else "START_REJECTED"})
        self.save()

    def status(self, entry):
        status = json.loads(self.request("pipelineStatus", {"json": "Y", "name": entry["name"], "id": entry["remote_id"]}))
        require(status.get("id") == entry["remote_id"] and status.get("pipelineName") == entry["name"], "remote identity mismatch")
        return status

    def finish(self, entry, success=True):
        for _ in range(60):
            status = self.status(entry)
            description = status.get("statusDescription", "")
            if any(word in description.lower() for word in ("finished", "stopped", "error", "failed")):
                errors = int((status.get("result") or {}).get("nrErrors", 0))
                ok = "finished" in description.lower() and errors == 0 and "error" not in description.lower()
                require(ok == success, "unexpected remote terminal outcome")
                entry["observations"].append({"state": "SUCCEEDED" if ok else "FAILED", "description": description, "errors": errors})
                self.save()
                return status
            time.sleep(0.5)
        raise RuntimeError("remote terminal timeout")

    def extract(self, attempt, expected=None):
        entry = self.register(attempt)
        self.start(entry)
        self.finish(entry)
        self.verify(entry, expected)
        return entry

    def verify(self, entry, expected=None):
        artifacts = []
        for table, (columns, _, count) in SLOTS.items():
            path = self.staging / entry["attempt"] / (table + "_raw.csv")
            content = path.read_bytes()
            require(not content.startswith(b"\xef\xbb\xbf") and b"\r\n" not in content, "CSV encoding/line contract")
            rows = list(csv.reader(io.StringIO(content.decode("utf-8"), newline=""), strict=True))
            values = (expected or {}).get(table, fixture(table))
            require(rows[0] == columns and len(rows) - 1 == count and rows[1:] == values, f"{table} field fidelity rejected")
            artifacts.append({"slot": table + "_raw", "path": entry["attempt"] + "/" + path.name,
                              "rows": count, "bytes": len(content), "sha256": sha(content), "columns": columns})
        manifest = {"contract": "postgres-ingestion-v1/B1-local", "source": "dpp_ingestion_source/dpp_source",
                    "batch": "fixture-quiescent-v1", "request_key": entry["request_key"],
                    "remote_id": entry["remote_id"], "template_sha256": self.report["template_sha256"],
                    "source_schema_sha256": self.preflight(), "slots": artifacts}
        entry["manifest_sha256"] = sha(canonical(manifest))
        (self.output / (entry["attempt"] + "-manifest.json")).write_bytes(canonical(manifest))
        return artifacts

    def fidelity(self):
        first = self.extract("baseline")
        again = self.extract("repeat")
        require([a["sha256"] for a in self.verify(first)] == [a["sha256"] for a in self.verify(again)], "nondeterministic CSV bytes")
        self.record("PGI-01", "real JDBC 6/10/14 rows; every original field equal; repeated extraction byte hashes equal")
        edge = '  中文,"quoted"\n第二行  '
        escaped = edge.replace("'", "''")
        self.psql(f"UPDATE dpp_source.enterprise SET registered_address='{escaped}' WHERE source_company_id='ENT-001';")
        expected = fixture("enterprise")
        expected[0][4] = edge
        self.extract("encoding-edge", {"enterprise": expected})
        self.load_fixtures()
        rejected = []
        mutations = [
            ("null", "ALTER TABLE dpp_source.enterprise ALTER COLUMN mobile DROP NOT NULL; UPDATE dpp_source.enterprise SET mobile=NULL WHERE source_company_id='ENT-001';", "UPDATE dpp_source.enterprise SET mobile='13800000001' WHERE source_company_id='ENT-001'; ALTER TABLE dpp_source.enterprise ALTER COLUMN mobile SET NOT NULL;"),
            ("extra-column", "ALTER TABLE dpp_source.energy ADD COLUMN surprise text;", "ALTER TABLE dpp_source.energy DROP COLUMN surprise;"),
            ("missing-column", "ALTER TABLE dpp_source.energy RENAME COLUMN energy_kwh TO wrong_name;", "ALTER TABLE dpp_source.energy RENAME COLUMN wrong_name TO energy_kwh;"),
            ("type-drift", "ALTER TABLE dpp_source.energy ALTER COLUMN energy_kwh TYPE numeric USING energy_kwh::numeric;", "ALTER TABLE dpp_source.energy ALTER COLUMN energy_kwh TYPE text USING energy_kwh::text;"),
            ("unknown-schema", "ALTER SCHEMA dpp_source RENAME TO unapproved_source;", "ALTER SCHEMA unapproved_source RENAME TO dpp_source;"),
        ]
        for label, mutation, restore in mutations:
            before = len(self.proxy.requests)
            self.psql(mutation)
            try:
                try:
                    self.register("reject-" + label)
                except AssertionError:
                    rejected.append(label)
                else:
                    raise AssertionError("schema drift dispatched")
                require(len(self.proxy.requests) == before, "rejected schema caused remote work")
            finally:
                self.psql(restore)
        self.load_fixtures()
        self.record("PGI-02", {"preserved": ["UTF-8", "quotes", "comma", "LF within field", "surrounding spaces", "empty string", "original dates", "negative energy"], "rejected_before_dispatch": rejected})

    def permissions(self):
        # Turn off transaction read-only to prove ACL denial, not just a setting.
        tests = {
            "insert": "INSERT INTO dpp_source.energy SELECT * FROM dpp_source.energy LIMIT 1;",
            "update": "UPDATE dpp_source.energy SET energy_kwh='0';",
            "delete": "DELETE FROM dpp_source.energy;", "truncate": "TRUNCATE dpp_source.energy;",
            "alter": "ALTER TABLE dpp_source.energy ADD COLUMN forbidden text;",
            "create-schema": "CREATE SCHEMA forbidden;",
            "create-table": "CREATE TABLE dpp_source.forbidden(id text);",
            "temporary-table": "CREATE TEMP TABLE forbidden(id text);",
            "private-table": "SELECT * FROM private_source.secret;",
            "owner-role": "SET ROLE fixture_owner;",
        }
        self.psql("CREATE SCHEMA private_source; CREATE TABLE private_source.secret(id text);")
        for label, sql in tests.items():
            result = self.psql("SET default_transaction_read_only=off; " + sql, reader=True, check=False)
            require(result.returncode != 0 and (b"permission denied" in result.stderr or b"must be owner" in result.stderr), f"ACL denial failed: {label}")
        role = self.psql("SELECT rolcanlogin,rolsuper,rolcreaterole,rolcreatedb,rolreplication,rolbypassrls FROM pg_roles WHERE rolname='dpp_reader';").stdout.decode().strip()
        require(role == "t|f|f|f|f|f", "reader has elevated privileges")
        require(self.psql("SELECT rolcanlogin FROM pg_roles WHERE rolname='fixture_owner';").stdout.decode().strip() == "f", "fixture writer can login")
        self.record("source-read-only", {"denied": list(tests), "reader_role": role, "fixture_owner": "NOLOGIN; source is quiescent except explicit test mutations"})

    def faults(self):
        # Provider registers; proxy discards the response. Caller has no run ID.
        self.proxy.drop = "registerPipeline"
        before = len(self.proxy.requests)
        try:
            self.register("register-loss")
        except (OSError, urllib.error.URLError, http.client.HTTPException):
            entry = self.report["attempts"][-1]
            entry["observations"].append({"state": "UNKNOWN", "reason": "REGISTER_RESPONSE_LOST_NO_ID"})
        else:
            raise AssertionError("register fault not injected")
        require("remote_id" not in entry and len(self.proxy.requests) == before + 1, "lost registration blindly retried")
        oracle_id = self.proxy.observed_ids[-1]
        require(oracle_id, "real registration did not occur")
        # Oracle proves only registration happened, not an extraction. Its ID is
        # never installed into caller recovery state.
        oracle = {"name": entry["name"], "remote_id": oracle_id}
        require(not self.status(oracle).get("executionStartDate"), "lost registration unexpectedly started")
        entry = self.register("start-loss")
        self.proxy.drop = "startPipeline"
        try:
            self.start(entry)
        except (OSError, urllib.error.URLError, http.client.HTTPException):
            entry["observations"].append({"state": "UNKNOWN", "reason": "START_RESPONSE_LOST"})
        else:
            raise AssertionError("start fault not injected")
        self.finish(entry)
        self.verify(entry)
        self.proxy.unavailable = True
        try:
            self.status(entry)
        except urllib.error.HTTPError as error:
            require(error.code == 503, "status fault not injected")
            entry["observations"].append({"state": "UNKNOWN", "reason": "STATUS_UNAVAILABLE"})
        else:
            raise AssertionError("status fault not injected")
        finally:
            self.proxy.unavailable = False
        self.finish(entry)
        # Source outage after successful registration: definite remote failure,
        # quarantined output; explicit new attempt after source returns.
        failed = self.register("source-down")
        self.docker("stop", "-t", "5", self.pg)
        self.start(failed, accepted=False)
        self.finish(failed, success=False)
        self.docker("start", self.pg)
        self.wait_pg()
        self.extract("source-recovered")
        # Restart destroys Hop's in-memory identities. Files alone do not prove
        # successful completion, and no start/register is replayed.
        pending = self.register("restart-pending")
        running = self.register("restart-running", slow=True)
        self.start(running)
        require("running" in self.status(running).get("statusDescription", "").lower(), "restart fault lacked active work")
        self.docker("restart", "-t", "5", self.hop)
        self.wait_hop()
        for subject in (entry, pending, running):
            try:
                self.status(subject)
            except (urllib.error.HTTPError, json.JSONDecodeError, AssertionError):
                subject["observations"].append({"state": "UNKNOWN", "reason": "HOP_RESTART_LOST_ID"})
            else:
                raise AssertionError("expected restart identity loss was not observed")
        self.verify(entry)  # complete original bytes remain, not a Core receipt
        try:
            self.verify(running)
        except (FileNotFoundError, AssertionError, IndexError):
            running["observations"].append({"state": "ARTIFACT_REJECTED", "reason": "INCOMPLETE_RESTART_OUTPUT"})
        else:
            raise AssertionError("active restart unexpectedly yielded a complete batch")
        self.extract("after-restart-explicit")
        for subject in self.report["attempts"]:
            starts = [request for request in self.proxy.requests if "/startPipeline?" in request["url"] and
                      urllib.parse.parse_qs(urllib.parse.urlsplit(request["url"]).query).get("name") == [subject["name"]]]
            require(len(starts) <= 1, "blind second start")
        self.record("PGI-04", "real register/start response loss; exact-ID query; status 503; source outage/new attempt; Hop restart loses IDs => UNKNOWN; zero blind second starts")

    def scan(self):
        corpus = []
        for file in HERE.rglob("*"):
            if file.is_file() and "__pycache__" not in file.parts:
                corpus.append(file.read_bytes())
        corpus.extend(canonical(request) for request in self.proxy.requests)
        for request in self.proxy.requests:
            if request["body"]:
                metadata = ET.fromstring(request["body"]).findtext("metastore_json")
                if metadata:
                    corpus.append(gzip.decompress(base64.b64decode(metadata)))
        corpus.extend(self.proxy.responses)
        for container in (self.pg, self.hop):
            corpus.append(self.docker("inspect", container).stdout)
            logs = self.docker("logs", container)
            corpus.extend([logs.stdout, logs.stderr])
        # Hop JSON loggingString is base64(gzip(log)); scan decoded form too.
        for response in self.proxy.responses:
            try:
                encoded = json.loads(response).get("loggingString")
                if encoded:
                    corpus.append(gzip.decompress(base64.b64decode(encoded)))
            except (ValueError, AttributeError, OSError):
                pass
        for file in self.output.rglob("*"):
            if file.is_file():
                corpus.append(file.read_bytes())
        corpus.append(canonical(self.report))
        candidates = self.passwords + [self.authorization.split(" ", 1)[1]]
        for secret in candidates:
            require(all(secret.encode() not in blob for blob in corpus), "SECRET_LEAK_DETECTED; evidence not exportable")
        # Engine container cannot mount the admin credential or any source data.
        mounts = json.loads(self.docker("inspect", self.hop).stdout)[0]["Mounts"]
        require(all(mount["Destination"] != "/run/secrets/admin-password" for mount in mounts), "admin credential exposed to engine")
        self.record("PGI-03", {"surfaces_scanned": ["URLs", "submitted XML", "metadata", "manifests", "all output artifacts", "Hop response/decoded log", "container logs", "Docker configured env/command"], "secret_values_found": 0, "source_account": "ACL negative checks passed", "admin_mount": "absent from Hop"})
        (self.output / "requests.json").write_bytes(canonical(self.proxy.requests))

    def save(self):
        (self.output / "report.json").write_bytes(canonical(self.report))

    def cleanup(self):
        if self.proxy:
            self.proxy.shutdown()
            self.proxy.server_close()
        if self.retain:
            for container in (self.hop, self.pg):
                self.docker("stop", "--time", "20", container, check=False)
            return  # Retained trial has no automatic destructive cleanup path.
        for container in (self.hop, self.pg):
            self.docker("rm", "-f", "-v", container, check=False)
        self.docker("network", "rm", self.network, check=False)
        require(self.private.resolve().parent == Path(tempfile.gettempdir()).resolve() and
                self.private.name.startswith("dpp-pgi-private-"), "unexpected cleanup path")
        shutil.rmtree(self.private)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True, help="new directory for non-secret B1 evidence")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    trial = Trial(output)
    try:
        trial.setup()
        trial.permissions()
        trial.fidelity()
        trial.faults()
        trial.scan()
        trial.report["result"] = "PASS"
    except Exception as error:
        trial.report["result"] = "FAIL"
        # No raw exception text: urllib/driver messages may carry credentials.
        trial.report["failure_type"] = type(error).__name__
        for container in (trial.pg, trial.hop):
            logs = trial.docker("logs", container, check=False)
            diagnostic = (logs.stdout + logs.stderr).decode(errors="replace")
            for secret in trial.passwords:
                diagnostic = diagnostic.replace(secret, "[REDACTED]")
            (output / (container + "-diagnostic.txt")).write_text(diagnostic, encoding="utf-8")
        print(f"FAIL {type(error).__name__}; diagnostic requires local inspection", flush=True)
        if isinstance(error, (AssertionError, RuntimeError)):
            print(str(error), flush=True)  # only locally authored safe messages
        raise SystemExit(1) from None
    finally:
        trial.save()
        trial.cleanup()
    print(f"B1 PASS; evidence: {output / 'report.json'}", flush=True)


if __name__ == "__main__":
    main()
