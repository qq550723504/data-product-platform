#!/usr/bin/env python3
"""Bounded PGI-09 synthetic catalog fixture; no discovery connector or Core writes."""
import argparse
import base64
import hashlib
import json
import re
import socket
import urllib.request
from pathlib import Path

RELEASE = "2.0.5-release"
COMPOSE_URL = f"https://github.com/open-metadata/OpenMetadata/releases/download/{RELEASE}/docker-compose-postgres.yml"
COMPOSE_SHA256 = "36d208a212cd62942199ad3ca4b11290b8536e5d1751a9a547e3738faedde3c2"


def prepare(output, project):
    import yaml  # Same PyYAML used to read the official Compose; no custom YAML parser.
    if not re.fullmatch(r"dpp-pgi09-[a-z0-9-]+", project):
        raise ValueError("a distinct dpp-pgi09-* project is required")
    output.mkdir(parents=True, exist_ok=False)
    with urllib.request.urlopen(COMPOSE_URL, timeout=30) as response:
        source = response.read()
    if hashlib.sha256(source).hexdigest() != COMPOSE_SHA256:
        raise ValueError("official Compose changed; review it before running")
    (output / "official-compose.yml").write_bytes(source)
    config = yaml.safe_load(source)
    names = ("postgresql", "elasticsearch", "execute-migrate-all", "openmetadata-server")
    services = {name: config["services"][name] for name in names}
    for name, service in services.items():
        service["container_name"] = project + "-" + name
        service["restart"] = "no"
        service.pop("ports", None)
    services["postgresql"]["volumes"] = ["om-db:/var/lib/postgresql/data"]
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    services["openmetadata-server"]["ports"] = [f"127.0.0.1:{port}:8585"]
    compose = {"name": project, "services": services, "volumes": {"om-db": {}, "es-data": {}}, "networks": {"app_net": {"driver": "bridge"}}}
    (output / "compose.json").write_text(json.dumps(compose, indent=2), encoding="utf-8")
    state = {"project": project, "url": f"http://127.0.0.1:{port}", "release": RELEASE, "official_sha256": COMPOSE_SHA256}
    (output / "state.json").write_text(json.dumps(state, indent=2), encoding="utf-8")
    print(json.dumps(state))


def catalog(output, seed):
    state = json.loads((output / "state.json").read_text(encoding="utf-8"))
    # Generated state cannot send credentials to a different destination.
    if not re.fullmatch(r"http://127\.0\.0\.1:[0-9]+", state["url"]):
        raise ValueError("isolated loopback OpenMetadata required")
    base = state["url"] + "/api"
    token = ""

    def request(path, body=None, method=None):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method or ("GET" if body is None else "PUT"))
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)

    version = request("/v1/system/version")
    if not seed:
        print(json.dumps(version))
        return
    if (output / "tables.json").exists() or (output / "token").exists():
        raise ValueError("seed only a fresh isolated fixture; do not reset an existing catalog")
    # Official quickstart account, only in this disposable loopback deployment.
    token = request("/v1/users/login", {"email": "admin@open-metadata.org", "password": base64.b64encode(b"admin").decode()}, "POST")["accessToken"]
    (output / "token").write_text(token, encoding="utf-8")
    from smoke import SLOTS
    service = request("/v1/services/databaseServices", {"name": "dpp_source", "serviceType": "Postgres", "description": "PGI-09 synthetic catalog authored through native API"})
    database = request("/v1/databases", {"name": "dpp_ingestion_source", "service": service["fullyQualifiedName"]})
    schema = request("/v1/databaseSchemas", {"name": "dpp_source", "database": database["fullyQualifiedName"]})
    owner = request("/v1/teams", {"name": "PGIOwners", "displayName": "PGI Synthetic Owners", "teamType": "Group"})
    steward = request("/v1/teams", {"name": "PGIStewards", "displayName": "PGI Synthetic Stewards", "teamType": "Group"})
    request("/v1/classifications", {"name": "PGISynthetic", "description": "Synthetic acceptance annotations"})
    request("/v1/tags", {"name": "Internal", "classification": "PGISynthetic", "description": "Synthetic catalog classification"})
    glossary = request("/v1/glossaries", {"name": "PGIPark", "description": "Synthetic park vocabulary"})
    term = request("/v1/glossaryTerms", {"name": "Activity", "glossary": glossary["fullyQualifiedName"], "description": "Synthetic enterprise activity"})
    table_type = request("/v1/metadata/types/name/table")
    references_type = request("/v1/metadata/types/name/entityReferenceList")
    request("/v1/metadata/types/" + table_type["id"], {
        "name": "dataStewards", "description": "Explicit catalog Steward assignments for the bounded PGI Adapter",
        "propertyType": {"id": references_type["id"], "type": "type"},
        "customPropertyConfig": {"config": ["user", "team"]},
    }, "PUT")
    request("/v1/domains", {"name": "SyntheticPark", "description": "Isolated synthetic release projections", "domainType": "Source-aligned"})
    tables = []
    for name, (columns, _, _) in SLOTS.items():
        table = request("/v1/tables", {
            "name": name, "databaseSchema": schema["fullyQualifiedName"], "description": "PGI-09 synthetic source v1",
            "columns": [{"name": column, "dataType": "TEXT"} for column in columns],
            "owners": [{"id": owner["id"], "type": "team"}],
            "tags": [
                {"tagFQN": "PGISynthetic.Internal", "source": "Classification", "labelType": "Manual", "state": "Confirmed"},
                {"tagFQN": term["fullyQualifiedName"], "source": "Glossary", "labelType": "Manual", "state": "Confirmed"},
            ],
            "extension": {"dataStewards": [{"id": steward["id"], "type": "team", "name": steward["name"], "displayName": steward["displayName"]}]} if name != "energy" else {},
        })
        tables.append({key: table[key] for key in ("id", "name", "fullyQualifiedName", "version")})
    (output / "tables.json").write_text(json.dumps(tables, indent=2), encoding="utf-8")
    print(json.dumps({"version": version, "tables": tables, "token": "retained server-side"}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("prepare", "health", "seed"))
    parser.add_argument("--output", type=Path, required=True, help="fresh ignored .data directory; contains a server-only token after seed")
    parser.add_argument("--project", default="dpp-pgi09-local")
    args = parser.parse_args()
    if args.operation == "prepare":
        prepare(args.output.resolve(), args.project)
    else:
        catalog(args.output.resolve(), args.operation == "seed")
