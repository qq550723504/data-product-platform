"""Controlled lifecycle contracts; no Docker service or native evidence fabrication."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import uuid
from types import SimpleNamespace
from unittest.mock import patch
import smoke

from trial_images import TrialImages
from smoke import Trial


class DockerModel:
    def __init__(self):
        self.images, self.refs, self.calls, self.consumers = {}, {}, [], []
        self.inspect_failure = self.consumer_failure = self.delete_failure = False
        self.lost_build_reply = self.change_before_delete = False
        self.inspect_count = 0

    def command(self, args, check=True, **kw):
        self.calls.append(args)
        code, out, error = 0, b"", b""
        if args[:3] == ["docker", "image", "inspect"]:
            self.inspect_count += 1
            ref = args[3]
            if self.inspect_failure:
                return subprocess.CompletedProcess(args, 1, b"", b"daemon unavailable")
            if self.change_before_delete and self.inspect_count == 2:
                self.images[self.refs[ref]]["RepoTags"].append("foreign:retained")
            identity = self.refs.get(ref, ref)
            if identity not in self.images:
                code, error = 1, ("Error response from daemon: No such image: " + ref).encode()
            else:
                out = json.dumps([self.images[identity]]).encode()
        elif args[:2] == ["docker", "build"]:
            tag = args[args.index("-t") + 1]
            labels = dict(args[i + 1].split("=", 1) for i, value in enumerate(args) if value == "--label")
            identity = "sha256:" + f"{len(self.images)+1:064x}"
            self.refs[tag] = identity
            canonical = tag.split(":", 1)[0] + "@" + identity
            self.refs[canonical] = identity
            self.images[identity] = dict(Id=identity, Config=dict(Labels=labels), RepoTags=[tag], RepoDigests=[canonical])
            if self.lost_build_reply:
                raise RuntimeError("controlled lost build reply")
        elif args[:4] == ["docker", "ps", "-a", "-q"]:
            code = int(self.consumer_failure)
            out = ("\n".join(self.consumers)).encode()
        elif args[:4] == ["docker", "image", "rm", "--no-prune"]:
            if self.delete_failure:
                code = 1
            else:
                identity = self.refs.pop(args[4])
                self.images.pop(identity)
        else:
            raise AssertionError("unexpected operation: " + str(args))
        return subprocess.CompletedProcess(args, code, out, error)

    def mutations(self):
        return [call for call in self.calls if call[:3] == ["docker", "image", "rm"]]


class ImageLifecycleContract(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="dpp320-contract-")
        self.directory = Path(self.temp.name)
        self.docker = DockerModel()
        self.owner = TrialImages(self.directory, str(uuid.uuid4()), False, self.docker.command)
        self.tag = self.owner.tag("minio")

    def tearDown(self):
        self.temp.cleanup()

    def build(self):
        self.identity = self.owner.build(self.tag, ["controlled-context"], 1)
        self.docker.inspect_count = 0
        return self.docker.images[self.identity]

    def result(self):
        return self.owner.cleanup()["references"][0]

    def test_owned_nonretained_release_and_repeat_are_idempotent(self):
        self.build()
        self.assertEqual(self.result()["reason"], "OWNED_REF_REMOVED")
        self.assertEqual(self.result()["reason"], "ALREADY_ABSENT")
        self.assertEqual(self.docker.mutations(), [["docker", "image", "rm", "--no-prune", self.tag]])
        self.assertNotIn(self.identity, self.docker.images)
        events = [json.loads(line)["event"] for line in self.owner.events.read_text().splitlines()]
        self.assertEqual(events, ["BUILD_INTENT", "BOUND", "DELETE_INTENT", "CLEANUP_OBSERVATION", "CLEANUP_OBSERVATION"])

    def test_retained_cleanup_has_no_docker_io(self):
        owner = TrialImages(self.directory, str(uuid.uuid4()), True, self.docker.command, stem="retained")
        owner.build(owner.tag("hop-jdbc"), ["controlled-context"], 1)
        self.docker.calls.clear()
        self.assertEqual(owner.cleanup()["status"], "RETAINED")
        self.assertEqual(self.docker.calls, [])

    def test_unknown_owner_repointed_and_additional_refs_are_preserved(self):
        for mutation in ("labels", "repoint", "tag", "digest", "metadata"):
            with self.subTest(mutation=mutation):
                directory = self.directory / mutation
                directory.mkdir()
                docker = DockerModel()
                owner = TrialImages(directory, str(uuid.uuid4()), False, docker.command)
                tag = owner.tag("minio")
                identity = owner.build(tag, ["context"], 1)
                image = docker.images[identity]
                if mutation == "labels": image["Config"]["Labels"] = {}
                elif mutation == "repoint": image["Id"] = "sha256:" + "f" * 64
                elif mutation == "tag": image["RepoTags"].append("foreign:retained")
                elif mutation == "digest": image["RepoDigests"].append("foreign@sha256:" + "f" * 64)
                else: image["Config"] = None
                self.assertIn(owner.cleanup()["status"], ("BLOCKED", "UNKNOWN"))
                self.assertEqual(docker.mutations(), [])
                self.assertIn(identity, docker.images)

    def test_stopped_consumer_blocks_and_is_recorded(self):
        self.build()
        self.docker.consumers = ["a" * 12]
        result = self.result()
        self.assertEqual(result["reason"], "CONTAINER_CONSUMER")
        self.assertEqual(result["consumer_ids"], self.docker.consumers)
        self.assertEqual(self.docker.mutations(), [])
        self.docker.consumers = []
        self.assertEqual(self.result()["status"], "PASS")

    def test_partial_build_reply_can_bind_its_owned_image(self):
        self.docker.lost_build_reply = True
        with self.assertRaises(RuntimeError): self.build()
        self.assertIsNone(self.owner.entries[0]["image_id"])
        self.assertEqual(self.result()["reason"], "OWNED_REF_REMOVED")
        events = [json.loads(line)["event"] for line in self.owner.events.read_text().splitlines()]
        self.assertIn("BOUND_AFTER_PARTIAL_SETUP", events)

    def test_partial_setup_no_image_is_not_created(self):
        self.build()
        self.docker.refs.clear(); self.docker.images.clear()
        self.owner.entries[0]["image_id"] = None
        self.assertEqual(self.result()["reason"], "NOT_CREATED")
        self.assertEqual(self.docker.mutations(), [])

    def test_legacy_store_without_canonical_refs_is_supported(self):
        base = self.docker.command
        def without_canonical(args, **kw):
            result = base(args, **kw)
            if args[:2] == ["docker", "build"]:
                self.docker.images[self.docker.refs[self.tag]]["RepoDigests"] = []
            return result
        self.owner.command = without_canonical
        self.build()
        self.assertEqual(self.owner.entries[0]["owned_digests"], [])
        self.assertEqual(self.result()["status"], "PASS")

    def test_missing_tag_does_not_authorize_id_deletion(self):
        self.build()
        del self.docker.refs[self.tag]
        self.assertEqual(self.result()["reason"], "MISSING_TAG_IMAGE_PRESENT")
        self.assertEqual(self.docker.mutations(), [])

    def test_daemon_or_consumer_error_does_not_authorize_deletion(self):
        self.build()
        for attr in ("inspect_failure", "consumer_failure"):
            setattr(self.docker, attr, True)
            self.assertEqual(self.result()["status"], "UNKNOWN")
            self.assertEqual(self.docker.mutations(), [])
            setattr(self.docker, attr, False)

    def test_changed_before_delete_and_delete_error_are_not_pass(self):
        self.build()
        self.docker.inspect_count = 0
        self.docker.change_before_delete = True
        self.assertEqual(self.result()["reason"], "CHANGED_BEFORE_DELETE")
        self.assertEqual(self.docker.mutations(), [])
        self.docker.images[self.identity]["RepoTags"] = [self.tag]
        self.docker.change_before_delete = False
        self.docker.delete_failure = True
        self.assertEqual(self.result()["status"], "FAILED")
        self.assertIn(self.tag, self.docker.refs)

    def test_existing_registry_or_tag_is_not_adopted(self):
        with self.assertRaises(FileExistsError):
            TrialImages(self.directory, self.owner.run_id, False, self.docker.command)
        self.build()
        with self.assertRaises(ValueError): self.build()
        with self.assertRaises(ValueError): self.owner.build("dpp-pgi-minio:2025-10-15", ["context"], 1)
        self.assertEqual(self.docker.mutations(), [])


    def test_trial_component_receipt_does_not_hide_image_failure(self):
        self.build()
        self.docker.consumers = ["a" * 12]
        with tempfile.TemporaryDirectory(prefix="dpp-pgi-private-") as private:
            trial = Trial.__new__(Trial)
            trial.proxy, trial.retain, trial.images, trial.report = None, False, self.owner, {}
            trial.hop, trial.pg, trial.network, trial.private = "controlled-hop", "controlled-pg", "controlled-net", Path(private)
            trial.docker = lambda *args, **kw: subprocess.CompletedProcess(args, 0, b"", b"")
            trial.cleanup()
            self.assertEqual(trial.report["cleanup"], "FAIL")
            self.assertEqual(trial.report["cleanup_detail"]["image_refs"]["status"], "BLOCKED")
            self.assertEqual(trial.report["cleanup_detail"]["temporary_credentials"]["status"], "PASS")
        self.assertEqual(self.docker.mutations(), [])

    def test_business_receipt_survives_cleanup_exception(self):
        output = self.directory / "main-output"
        report = {}
        def cleanup():
            report["cleanup"] = "FAIL"
            raise RuntimeError("controlled cleanup failure")
        def save():
            (output / "report.json").write_text(json.dumps(report))
        trial = SimpleNamespace(setup=lambda: None, permissions=lambda: None,
            fidelity=lambda: None, faults=lambda: None, scan=lambda: None,
            report=report, cleanup=cleanup, save=save)
        with patch.object(smoke, "Trial", return_value=trial), patch("sys.argv", ["smoke", "--output", str(output)]):
            with self.assertRaisesRegex(RuntimeError, "controlled cleanup failure"):
                smoke.main()
        self.assertEqual(json.loads((output / "report.json").read_text()), {"result":"PASS", "cleanup":"FAIL"})

    def test_trial_timeout_is_observed_not_not_run(self):
        self.build()
        with tempfile.TemporaryDirectory(prefix="dpp-pgi-private-") as private:
            trial = Trial.__new__(Trial)
            trial.proxy, trial.retain, trial.images, trial.report = None, False, self.owner, {}
            trial.hop, trial.pg, trial.network, trial.private = "controlled-hop", "controlled-pg", "controlled-net", Path(private)
            def failure(*args, **kw):
                raise subprocess.TimeoutExpired(args, 1)
            trial.docker = failure
            trial.cleanup()
            detail = trial.report["cleanup_detail"]
            self.assertEqual(trial.report["cleanup"], "FAIL")
            self.assertEqual(detail["networks"]["status"], "UNKNOWN")
            self.assertEqual(detail["containers"]["attempts"][0]["failure_type"], "TimeoutExpired")
            self.assertNotEqual(detail["image_refs"]["status"], "NOT_RUN")


if __name__ == "__main__":
    unittest.main()
