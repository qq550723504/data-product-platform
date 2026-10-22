"""Reuse D1's read-only mount admission before starting retained trial services."""
import json, os, subprocess
from pathlib import Path
from smoke import command, require, HERE, REPO

class DurableStorage:
    def __init__(self, output, private, helper):
        self.output, self.private, self.helper = output, private, helper
        require(helper.is_file(), "D1 durable-storage helper required before retained startup")
        self.services, self.checks = [], []

    def check(self, services, run_id, state="running"):
        items=[]
        for service in services:
            item=json.loads(command(["docker","inspect",service["name"]]).stdout)[0]
            items.append(dict(service,container=item))
        data={"run_id":run_id,"volumes":[s["volume"] for s in services],"services":items}
        path=self.private/"storage-inspect.json";path.write_text(json.dumps(data),encoding="utf-8")
        result=command(["pwsh","-NoProfile","-File",str(HERE/"retained-storage.ps1"),"-Helper",str(self.helper),"-InputPath",str(path),"-AllowedStates",state])
        report=json.loads(result.stdout.decode("utf-8-sig"));self.checks.append(report)
        (self.output/"durable-storage.json").write_text(json.dumps(self.checks,indent=2),encoding="utf-8")
        return report

    def start(self, name, path, volume, role, run_id):
        service=dict(name=name,path=path,volume=volume,role=role)
        require(all(s["name"]!=name and s["volume"]!=volume and s["role"]!=role for s in self.services),"distinct retained services required")
        self.services.append(service)
        (self.output/"storage-registry.json").write_text(json.dumps({"run_id":run_id,"services":self.services},indent=2),encoding="utf-8")
        state=json.loads(command(["docker","inspect",name]).stdout)[0]["State"]["Status"]
        require(state in ("created","exited"),"new or explicitly stopped retained service required")
        self.check([service],run_id,state)
        command(["docker","start",name]);self.check([service],run_id)

def private_directory(prefix):
    import tempfile
    base=Path(os.environ.get("LOCALAPPDATA",str(Path.home()/".local/share")))/"Codex/retained-trials"
    base.mkdir(parents=True,exist_ok=True)
    path=Path(tempfile.mkdtemp(prefix=prefix,dir=base))
    if os.name=="nt":
        result=command(["whoami","/user","/fo","csv","/nh"])
        import csv,io
        sid=next(csv.reader(io.StringIO(result.stdout.decode())))[1]
        command(["icacls",str(path),"/inheritance:r","/grant:r","*"+sid+":(OI)(CI)F","*S-1-5-18:(OI)(CI)F"])
    else:path.chmod(0o700)
    return path
