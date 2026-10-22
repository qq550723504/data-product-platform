"""C2 uses the existing isolated B3 runtime; allocates explicit private subnets."""
import argparse, ipaddress, json
from pathlib import Path
import smoke, b2_smoke, b3_runtime

def run(output, metadata_output, cache, source_subnet="10.233.21.0/24", storage_subnet="10.233.22.0/24", durable_helper=None, binary_prefix="pgi-b3-c2", migrations=None):
    original = smoke.command
    proposed=[ipaddress.ip_network(source_subnet),ipaddress.ip_network(storage_subnet)]
    smoke.require(not proposed[0].overlaps(proposed[1]),"trial subnets overlap")
    ids=original(["docker","network","ls","-q"]).stdout.decode().split()
    if ids:
        existing=json.loads(original(["docker","network","inspect",*ids]).stdout)
        for item in existing:
            for cfg in item.get("IPAM",{}).get("Config",[]) or []:
                if cfg.get("Subnet"):
                    allocated=ipaddress.ip_network(cfg["Subnet"])
                    smoke.require(all(p.version!=allocated.version or not p.overlaps(allocated) for p in proposed),"trial subnet overlaps existing network")
    subnets = iter([source_subnet, storage_subnet])
    def command(args, *pos, **kw):
        if args[:3] == ["docker", "network", "create"]:
            args = args[:3] + ["--subnet", next(subnets)] + args[3:]
        return original(args, *pos, **kw)
    smoke.command = b2_smoke.command = b3_runtime.command = command
    state = json.loads((metadata_output / "state.json").read_text())
    b3_runtime.run(output, cache, binary_prefix, state["url"] + "/api",
        metadata_output / "admin-token", "park/remediation/energy-quarantine-v1.yaml",durable_helper.resolve() if durable_helper else None,migrations.resolve() if migrations else None)

if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--output",type=Path,required=True)
    p.add_argument("--metadata-output",type=Path,required=True)
    p.add_argument("--cache",type=Path,required=True)
    p.add_argument("--source-subnet",default="10.233.21.0/24");p.add_argument("--storage-subnet",default="10.233.22.0/24")
    p.add_argument("--durable-helper",type=Path);p.add_argument("--binary-prefix",default="pgi-b3-c2")
    p.add_argument("--migrations",type=Path)
    a=p.parse_args(); run(a.output.resolve(), a.metadata_output.resolve(), a.cache.resolve(),a.source_subnet,a.storage_subnet,a.durable_helper,a.binary_prefix,a.migrations)
