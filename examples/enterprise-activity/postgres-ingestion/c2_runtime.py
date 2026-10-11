"""C2 uses the existing isolated B3 runtime; allocates explicit private subnets."""
import argparse, json
from pathlib import Path
import smoke, b2_smoke, b3_runtime

def run(output, metadata_output, cache):
    original = smoke.command
    subnets = iter(["10.233.21.0/24", "10.233.22.0/24"])
    def command(args, *pos, **kw):
        if args[:3] == ["docker", "network", "create"]:
            args = args[:3] + ["--subnet", next(subnets)] + args[3:]
        return original(args, *pos, **kw)
    smoke.command = b2_smoke.command = b3_runtime.command = command
    state = json.loads((metadata_output / "state.json").read_text())
    b3_runtime.run(output, cache, "pgi-b3-c2", state["url"] + "/api",
        metadata_output / "admin-token", "park/remediation/energy-quarantine-v1.yaml")

if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--output",type=Path,required=True)
    p.add_argument("--metadata-output",type=Path,required=True)
    p.add_argument("--cache",type=Path,required=True)
    a=p.parse_args(); run(a.output.resolve(), a.metadata_output.resolve(), a.cache.resolve())
