"""Native validation on fresh scratch images only; no existing cleanup."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import uuid

from trial_images import TrialImages


def command(args, check=True, **kw):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=kw.get("timeout", 60))
    if check and result.returncode:
        raise RuntimeError("native operation failed; raw diagnostics withheld")
    return result


def inventory():
    ids = sorted(set(command(["docker", "image", "ls", "-a", "-q", "--no-trunc"]).stdout.decode().split()))
    images = {}
    for identity in ids:
        info = json.loads(command(["docker", "image", "inspect", identity]).stdout)[0]
        images[identity] = dict(tags=sorted(info.get("RepoTags") or []), digests=sorted(info.get("RepoDigests") or []))
    state = {}
    for identity in command(["docker", "ps", "-a", "-q", "--no-trunc"]).stdout.decode().split():
        info = json.loads(command(["docker", "inspect", identity]).stdout)[0]
        state[identity] = dict(image=info["Image"], configured_image=info["Config"]["Image"], status=info["State"]["Status"],
            mounts=[dict(type=m["Type"], name=m.get("Name"), destination=m["Destination"]) for m in info["Mounts"]])
    return dict(images=images, containers=state,
        volumes=sorted(command(["docker", "volume", "ls", "-q"]).stdout.decode().split()))


def run(output):
    output.mkdir(parents=True, exist_ok=False)
    context = output / "scratch-context"; context.mkdir()
    (context / "Dockerfile").write_text("FROM scratch\nCOPY payload /fixture\n", encoding="utf-8", newline="\n")
    (context / "payload").write_bytes(b"synthetic image lifecycle fixture\n")
    before = inventory()
    checks, owned, retained = [], [], []

    def make(name, retain=False, callback=command):
        directory = output / name; directory.mkdir()
        owner = TrialImages(directory, str(uuid.uuid4()), retain, callback)
        return owner, owner.tag("minio")

    owner, tag = make("release")
    identity = owner.build(tag, [str(context)], 120)
    owned.append(dict(tag=tag, image_id=identity, run_id=owner.run_id))
    first, repeat = owner.cleanup(), owner.cleanup()
    assert first["status"] == repeat["status"] == "PASS"
    assert first["references"][0]["image_state"] == "ABSENT"
    assert repeat["references"][0]["reason"] == "ALREADY_ABSENT"
    checks.append("owned non-retained release and retry")

    owner, tag = make("retain", retain=True)
    identity = owner.build(tag, [str(context)], 120)
    assert owner.cleanup()["status"] == "RETAINED" and owner.inspect(tag)["Id"] == identity
    retained.append(dict(tag=tag, image_id=identity, reason="retain"))
    checks.append("retain has no image deletion")

    owner, tag = make("consumer")
    identity = owner.build(tag, [str(context)], 120)
    name = "dpp320-image-consumer-" + uuid.uuid4().hex
    container = command(["docker", "create", "--name", name, "--label", "org.dpp.trial.run=" + owner.run_id,
        "--entrypoint", "/never-started", identity]).stdout.decode().strip()
    info = json.loads(command(["docker", "inspect", container]).stdout)[0]
    assert info["Id"] == container and info["Image"] == identity and info["Mounts"] == []
    assert info["Config"]["Labels"]["org.dpp.trial.run"] == owner.run_id
    assert owner.cleanup()["references"][0]["reason"] == "CONTAINER_CONSUMER"
    assert owner.inspect(tag)["Id"] == identity
    # Only this exact newly created, never-started, mount-free container.
    command(["docker", "container", "rm", container])
    assert owner.cleanup()["status"] == "PASS"
    owned.append(dict(tag=tag, image_id=identity, run_id=owner.run_id, consumer=container))
    checks.append("created/stopped consumer blocks; own consumer removed before release")

    owner, tag = make("additional-ref")
    identity = owner.build(tag, [str(context)], 120)
    alias = "dpp320-test-alias:trial-" + uuid.uuid4().hex
    command(["docker", "tag", identity, alias])
    assert owner.cleanup()["references"][0]["reason"] == "ADDITIONAL_REFERENCES"
    assert owner.inspect(alias)["Id"] == identity
    retained.append(dict(tag=tag, alias=alias, image_id=identity, reason="additional-reference"))
    checks.append("additional ref blocks; both fresh refs retained")

    def lost_reply(args, **kw):
        result = command(args, **kw)
        if args[:2] == ["docker", "build"]:
            raise RuntimeError("controlled response-loss after successful native build")
        return result
    owner, tag = make("partial-build", callback=lost_reply)
    try:
        owner.build(tag, [str(context)], 120)
    except RuntimeError:
        pass
    else:
        raise AssertionError("response-loss injection not exercised")
    assert owner.entries[0]["image_id"] is None
    assert owner.cleanup()["status"] == "PASS"
    owned.append(dict(tag=tag, image_id=owner.entries[0]["image_id"], run_id=owner.run_id))
    checks.append("native completed build with controlled lost reply recovers ownership")

    after = inventory()
    for identity, info in before["images"].items():
        assert after["images"].get(identity) == info, "existing image refs changed"
    for identity, info in before["containers"].items():
        assert after["containers"].get(identity) == info, "existing container changed"
    assert before["volumes"] == after["volumes"], "volume inventory changed"
    canonical = json.dumps(before, sort_keys=True, separators=(",", ":")).encode()
    report = dict(status="PASS", checks=checks, owned_removed=owned, fresh_retained=retained,
        existing_image_count=len(before["images"]), existing_container_count=len(before["containers"]),
        existing_volume_count=len(before["volumes"]), existing_inventory_sha256=hashlib.sha256(canonical).hexdigest(),
        existing_objects_unchanged=True, existing_cleanup_calls=0, volume_mutations=0, backup_calls=0,
        business_runtime_acceptance="NOT_RUN", human_explanation="DEFERRED")
    (output / "native-image-lifecycle.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(json.dumps(dict(status=report["status"], checks=len(checks), existing_objects_unchanged=True)))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    run(parser.parse_args().output.resolve())
