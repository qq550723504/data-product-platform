"""Archive completed, explicitly owned Docker helpers; never remove data or services.

Docker remains the runtime and collector. There is no daemon, global prune, TTL
guess, forced removal, or volume/image/network cleanup here.
"""
import argparse
import csv
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import uuid


PREFIX = "org.dpp.temporary."
REPO = Path(__file__).resolve().parents[1]


class CleanupError(RuntimeError):
    pass


def command(args, **kwargs):
    result = subprocess.run(args, capture_output=True, timeout=60, **kwargs)
    if result.returncode:
        # Docker errors and helper output can contain credentials.
        raise CleanupError("Docker operation failed; private resources retained")
    return result


def write_json(path, value):
    temporary = path.with_suffix(".tmp")
    with temporary.open("w", encoding="utf-8", newline="\n") as stream:
        json.dump(value, stream, indent=2, sort_keys=True)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    temporary.replace(path)


def private_root(path=None):
    if path is None:
        base = Path(os.environ["LOCALAPPDATA"]) / "Codex" if os.name == "nt" else Path.home() / ".local/state/dpp"
        path = base / "docker-helper-archives"
    root = Path(path).resolve()
    if root == REPO or REPO in root.parents or any((p / ".git").exists() for p in (root, *root.parents)):
        raise CleanupError("Helper diagnostics must be outside Git worktrees")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    if os.name == "nt":
        result = command(["whoami", "/user", "/fo", "csv", "/nh"])
        sid = next(csv.reader(io.StringIO(result.stdout.decode())))[1]
        if not re.fullmatch(r"S-1-[0-9-]+", sid):
            raise CleanupError("Cannot establish private archive ownership")
        command(["icacls", str(root), "/inheritance:r", "/grant:r",
                 "*" + sid + ":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F"])
    else:
        root.chmod(0o700)
    return root


def identity(item):
    return {key: item[key] for key in ("Id", "Name", "Image", "Created", "State", "RestartCount", "Config", "HostConfig", "Mounts")}


def stopped_helper(item, expected):
    state, host = item["State"], item["HostConfig"]
    if (state["Status"] != "exited" or state["Running"] or state["ExitCode"] != expected
            or state.get("OOMKilled") or state.get("Error") or state["FinishedAt"].startswith("0001-")):
        raise CleanupError("Helper has no confirmed expected completion")
    if (not host["ReadonlyRootfs"] or host["RestartPolicy"]["Name"] not in ("", "no")
            or host.get("PortBindings") or host.get("AutoRemove") or host.get("Privileged")
            or any(m["Type"] == "volume" for m in item["Mounts"])
            or (item["Config"].get("Labels") or {}).get("com.docker.compose.project")):
        raise CleanupError("Service, data mount, or unmanaged helper retained")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def archive_and_remove(item, folder, docker=command):
    """Called only with an owner's confirmed completion or an audited exact list.

    Full diagnostics stay private. Ordinary Docker rm refuses a concurrent start;
    single-writer ownership forbids reuse after the completion receipt.
    """
    container_id = item["Id"]
    if not re.fullmatch(r"[0-9a-f]{64}", container_id):
        raise CleanupError("Full immutable container ID required")
    folder.mkdir(mode=0o700, exist_ok=True)
    if (folder / "metadata.json").exists():
        saved = json.loads((folder / "metadata.json").read_text(encoding="utf-8"))
        if identity(saved) != identity(item):
            raise CleanupError("Partial archive belongs to another completion")
    write_json(folder / "metadata.json", item)
    logs = docker(["docker", "logs", "--timestamps", container_id])
    for name, payload in (("stdout.log", logs.stdout), ("stderr.log", logs.stderr)):
        temporary = folder / (name + ".tmp")
        with temporary.open("wb") as stream:
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(folder / name)
    hashes = {name: digest(folder / name) for name in ("metadata.json", "stdout.log", "stderr.log")}
    write_json(folder / "archive.json", {"id": container_id, "sha256": hashes})
    return remove_archived(folder, docker)


def remove_archived(folder, docker=command):
    archive = json.loads((folder / "archive.json").read_text(encoding="utf-8"))
    for name in ("metadata.json", "stdout.log", "stderr.log"):
        if digest(folder / name) != archive["sha256"][name]:
            raise CleanupError("Archive verification failed; container retained")
    previous = json.loads((folder / "metadata.json").read_text(encoding="utf-8"))
    container_id = previous["Id"]
    if container_id != archive["id"] or not re.fullmatch(r"[0-9a-f]{64}", container_id):
        raise CleanupError("Archive identity mismatch")
    present = docker(["docker", "container", "ls", "--all", "--quiet", "--no-trunc", "--filter", "id=" + container_id]).stdout.split()
    if present:
        current = json.loads(docker(["docker", "container", "inspect", container_id]).stdout)[0]
        if identity(current) != identity(previous):
            raise CleanupError("Container changed after completion; retained")
        # NEVER add --force or --volumes. All mounts and backing files survive.
        docker(["docker", "container", "rm", container_id])
        remaining = docker(["docker", "container", "ls", "--all", "--quiet", "--no-trunc", "--filter", "id=" + container_id]).stdout
        if remaining.strip():
            raise CleanupError("Container removal not confirmed")
    result = {"status": "REMOVED" if present else "ALREADY_ABSENT", "id": container_id,
              "volumes_removed": 0, "images_removed": 0, "networks_removed": 0}
    write_json(folder / "removal.json", result)
    return result


class TemporaryContainers:
    def __init__(self, root=None, docker=command):
        self.root, self.docker = private_root(root), docker

    def create(self, run_id, arguments):
        run_id = str(uuid.UUID(run_id))
        owner = uuid.uuid4().hex
        folder = self.root / owner
        folder.mkdir(mode=0o700)
        ticket = folder / "ticket.json"
        labels = {PREFIX + "kind": "helper", PREFIX + "run": run_id, PREFIX + "owner": owner}
        record = {"run_id": run_id, "owner": owner, "state": "CREATE_PENDING", "labels": labels}
        write_json(ticket, record)
        opts = [part for key, value in labels.items() for part in ("--label", key + "=" + value)]
        result = self.docker(["docker", "create", *opts, *arguments])
        container_id = result.stdout.decode().strip()
        if not re.fullmatch(r"[0-9a-f]{64}", container_id):
            raise CleanupError("Docker returned no full container ID; creation retained")
        record.update(id=container_id, state="CREATED")
        write_json(ticket, record)
        return {"id": container_id, "ticket": str(ticket)}

    def load(self, ticket):
        ticket = Path(ticket).resolve()
        if (ticket.name != "ticket.json" or ticket.parent.parent != self.root
                or not re.fullmatch(r"[0-9a-f]{32}", ticket.parent.name)):
            raise CleanupError("Ticket is outside this private registry")
        record = json.loads(ticket.read_text(encoding="utf-8"))
        if (record["owner"] != ticket.parent.name or not re.fullmatch(r"[0-9a-f]{32}", record["owner"])
                or str(uuid.UUID(record["run_id"])) != record["run_id"]):
            raise CleanupError("Invalid owner ticket")
        expected = {PREFIX + "kind": "helper", PREFIX + "run": record["run_id"], PREFIX + "owner": record["owner"]}
        if record["labels"] != expected:
            raise CleanupError("Ticket ownership mismatch")
        return ticket, record

    def complete(self, ticket, expected=0, retain=False):
        ticket, record = self.load(ticket)
        if record["state"] == "REMOVED":
            return {"status": "ALREADY_REMOVED", "id": record["id"]}
        if record["state"] != "CREATED":
            raise CleanupError("Only the creating owner can confirm completion")
        item = json.loads(self.docker(["docker", "container", "inspect", record["id"]]).stdout)[0]
        if item["Id"] != record["id"] or any((item["Config"].get("Labels") or {}).get(k) != v for k, v in record["labels"].items()):
            raise CleanupError("Live helper ownership mismatch")
        stopped_helper(item, expected)
        record.update(state="RETAINED" if retain else "COMPLETED", expected_exit=expected,
                      completed_identity=identity(item))
        write_json(ticket, record)
        if retain:
            return {"status": "RETAINED", "id": record["id"]}
        return self.collect(ticket)

    def collect(self, ticket):
        ticket, record = self.load(ticket)
        if record["state"] != "COMPLETED":
            return {"status": "RETAINED", "reason": "NO_COMPLETION", "id": record.get("id")}
        folder = ticket.parent / "diagnostics"
        if (folder / "archive.json").exists():
            # Resume only a fully archived completion; no new snapshot is adopted.
            archived = json.loads((folder / "metadata.json").read_text(encoding="utf-8"))
            if identity(archived) != record["completed_identity"]:
                raise CleanupError("Completion archive identity mismatch")
            stopped_helper(archived, record["expected_exit"])
            result = remove_archived(folder, self.docker)
        else:
            item = json.loads(self.docker(["docker", "container", "inspect", record["id"]]).stdout)[0]
            if identity(item) != record["completed_identity"]:
                raise CleanupError("Completed helper changed; retained")
            result = archive_and_remove(item, folder, self.docker)
        record["state"] = "REMOVED"
        write_json(ticket, record)
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive-root")
    sub = parser.add_subparsers(dest="action", required=True)
    create = sub.add_parser("create")
    create.add_argument("--run-id", required=True)
    create.add_argument("arguments", nargs=argparse.REMAINDER)
    complete = sub.add_parser("complete")
    complete.add_argument("--ticket", required=True)
    complete.add_argument("--expected-exit", type=int, default=0)
    complete.add_argument("--retain", action="store_true")
    collect = sub.add_parser("collect")
    collect.add_argument("--run-id", required=True)
    args = parser.parse_args()
    helpers = TemporaryContainers(args.archive_root)
    if args.action == "create":
        arguments = args.arguments[1:] if args.arguments[:1] == ["--"] else args.arguments
        if not arguments:
            raise CleanupError("Docker create arguments required")
        result = helpers.create(args.run_id, arguments)
    elif args.action == "complete":
        result = helpers.complete(args.ticket, args.expected_exit, args.retain)
    else:
        run_id = str(uuid.UUID(args.run_id))
        result = []
        for ticket in helpers.root.glob("*/ticket.json"):
            _, record = helpers.load(ticket)
            if record["run_id"] == run_id and record["state"] == "COMPLETED":
                result.append(helpers.collect(ticket))
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except (CleanupError, OSError, ValueError, KeyError, subprocess.SubprocessError):
        # Do not publish command arguments, raw Docker output, or private paths.
        print(json.dumps({"status": "UNKNOWN", "reason": "CLEANUP_INCOMPLETE_RESOURCES_RETAINED"}))
        raise SystemExit(1)
