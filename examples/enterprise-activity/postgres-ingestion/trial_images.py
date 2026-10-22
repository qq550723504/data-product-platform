"""Finite, single-writer trial image ownership; never adopts existing images."""
import json
import os
from pathlib import Path
import re
import uuid


class InspectionError(RuntimeError):
    pass


class TrialImages:
    def __init__(self, output, run_id, retain, command, stem="image-ownership"):
        self.run_id = str(uuid.UUID(run_id))
        if type(retain) is not bool:
            raise ValueError("explicit image retention required")
        self.retain, self.command = retain, command
        self.owner = uuid.uuid4().hex
        self.entries = []
        self.registry = Path(output) / (stem + ".json")
        self.events = Path(output) / (stem + "-events.jsonl")
        # Exclusive creation: a new process cannot adopt an old run registry.
        with self.registry.open("x", encoding="utf-8", newline="\n") as stream:
            stream.write(json.dumps(self.snapshot()) + "\n")
        with self.events.open("x", encoding="utf-8", newline="\n"):
            pass

    @property
    def labels(self):
        return {"org.dpp.trial.run": self.run_id, "org.dpp.trial.image-owner": self.owner,
                "org.dpp.trial.retention": "retain" if self.retain else "non-retain"}

    def tag(self, kind):
        if kind not in ("minio", "hop-jdbc"):
            raise ValueError("unsupported trial image kind")
        return "dpp-pgi-" + kind + ":trial-" + uuid.UUID(self.run_id).hex

    def snapshot(self):
        return {"run_id": self.run_id, "owner": self.owner, "retain": self.retain,
                "images": self.entries}

    def record(self, event, entry, **detail):
        with self.events.open("a", encoding="utf-8", newline="\n") as stream:
            stream.write(json.dumps(dict(event=event, tag=entry["tag"],
                image_id=entry["image_id"], **detail), sort_keys=True) + "\n")
            stream.flush()
            os.fsync(stream.fileno())
        temporary = self.registry.with_suffix(".tmp")
        temporary.write_text(json.dumps(self.snapshot(), indent=2) + "\n", encoding="utf-8", newline="\n")
        temporary.replace(self.registry)

    def inspect(self, ref):
        try:
            result = self.command(["docker", "image", "inspect", ref], check=False)
        except Exception:
            raise InspectionError("image inspection failed") from None
        if result.returncode:
            missing = ("Error response from daemon: No such image: " + ref).encode()
            if result.returncode == 1 and result.stderr.strip() == missing:
                return None
            raise InspectionError("image inspection failed; raw diagnostics withheld")
        try:
            items = json.loads(result.stdout)
            image = items[0]
            if len(items) != 1 or not re.fullmatch(r"sha256:[0-9a-f]{64}", image["Id"]):
                raise ValueError()
            if not isinstance(image.get("Config"), dict) or not isinstance(image["Config"].get("Labels") or {}, dict):
                raise ValueError()
            if not isinstance(image.get("RepoTags"), list) or not isinstance(image.get("RepoDigests"), list):
                raise ValueError()
            return image
        except (ValueError, KeyError, IndexError, TypeError):
            raise InspectionError("invalid image inspection") from None

    def owned(self, image):
        return all((image.get("Config", {}).get("Labels") or {}).get(k) == v
                   for k, v in self.labels.items())

    def bind(self, entry, image, event):
        digests = image["RepoDigests"]
        prefix = entry["tag"].split(":", 1)[0] + "@sha256:"
        for ref in digests:
            if not isinstance(ref, str) or not ref.startswith(prefix) or not re.fullmatch(r"[0-9a-f]{64}", ref[len(prefix):]):
                raise InspectionError("built image has foreign canonical reference")
            resolved = self.inspect(ref)
            if resolved is None or resolved["Id"] != image["Id"] or not self.owned(resolved):
                raise InspectionError("canonical reference identity mismatch")
        entry["image_id"], entry["owned_digests"] = image["Id"], list(digests)
        self.record(event, entry, owned_digests=entry["owned_digests"])

    def build(self, tag, arguments, timeout):
        if tag not in (self.tag("minio"), self.tag("hop-jdbc")):
            raise ValueError("image tag is outside this run")
        if any(e["tag"] == tag for e in self.entries) or self.inspect(tag) is not None:
            raise ValueError("trial image tag must be fresh")
        entry = {"tag": tag, "image_id": None, "owned_digests": []}
        self.entries.append(entry)
        self.record("BUILD_INTENT", entry)
        labels = [part for k, v in self.labels.items() for part in ("--label", k + "=" + v)]
        self.command(["docker", "build", *labels, "-t", tag, *arguments], timeout=timeout)
        image = self.inspect(tag)
        if image is None or not self.owned(image):
            raise InspectionError("built image ownership mismatch")
        self.bind(entry, image, "BOUND")
        return entry["image_id"]

    def release(self, entry):
        tag, expected = entry["tag"], entry["image_id"]
        image = self.inspect(tag)
        if image is None:
            if expected is None:
                return dict(status="PASS", reason="NOT_CREATED", tag_state="ABSENT", image_state="NOT_BOUND")
            remaining = self.inspect(expected)
            if remaining is not None:
                return dict(status="BLOCKED", reason="MISSING_TAG_IMAGE_PRESENT", tag_state="ABSENT", image_state="PRESENT")
            return dict(status="PASS", reason="ALREADY_ABSENT", tag_state="ABSENT", image_state="ABSENT")
        if not self.owned(image):
            return dict(status="BLOCKED", reason="FOREIGN_OWNERSHIP")
        if expected is not None and image["Id"] != expected:
            return dict(status="BLOCKED", reason="REPOINTED")
        if expected is None:
            # Build may have completed before its return/inspection failed.
            self.bind(entry, image, "BOUND_AFTER_PARTIAL_SETUP")
            expected = entry["image_id"]
        if image["RepoTags"] != [tag] or image["RepoDigests"] != entry["owned_digests"]:
            return dict(status="BLOCKED", reason="ADDITIONAL_REFERENCES")
        try:
            consumers = self.command(["docker", "ps", "-a", "-q", "--filter", "ancestor=" + expected], check=False)
        except Exception:
            raise InspectionError("consumer inspection failed") from None
        if consumers.returncode:
            raise InspectionError("consumer inspection failed")
        if consumers.stdout.strip():
            return dict(status="BLOCKED", reason="CONTAINER_CONSUMER",
                        consumer_ids=consumers.stdout.decode().split())
        # Single-writer run namespace, not a Docker-wide compare/delete fence.
        current = self.inspect(tag)
        if current != image:
            return dict(status="BLOCKED", reason="CHANGED_BEFORE_DELETE")
        self.record("DELETE_INTENT", entry)
        try:
            result = self.command(["docker", "image", "rm", "--no-prune", tag], check=False)
        except Exception:
            raise InspectionError("delete outcome unknown") from None
        after = self.inspect(tag)
        remaining = self.inspect(expected)
        if result.returncode:
            return dict(status="FAILED", reason="DELETE_FAILED",
                        tag_state="ABSENT" if after is None else "PRESENT",
                        image_state="ABSENT" if remaining is None else "PRESENT")
        if after is not None:
            return dict(status="FAILED", reason="TAG_STILL_PRESENT")
        return dict(status="PASS", reason="OWNED_REF_REMOVED", tag_state="ABSENT",
                    image_state="ABSENT" if remaining is None else "PRESENT")

    def cleanup(self):
        decisions = []
        for entry in self.entries:
            if self.retain:
                decision = dict(status="RETAINED", reason="EXPLICIT_RETENTION")
            else:
                try:
                    decision = self.release(entry)
                except InspectionError:
                    decision = dict(status="UNKNOWN", reason="INSPECTION_FAILED")
            self.record("CLEANUP_OBSERVATION", entry, **decision)
            decisions.append(dict(entry, **decision))
        status = "RETAINED" if self.retain else "PASS"
        for candidate in ("BLOCKED", "FAILED", "UNKNOWN"):
            if any(d["status"] == candidate for d in decisions):
                status = candidate
        return dict(status=status, references=decisions)
