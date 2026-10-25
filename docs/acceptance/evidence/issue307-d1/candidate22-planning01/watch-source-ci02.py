"""Preserve this exact source PR CI's completion; not execution authority."""
import datetime as dt
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
OUT=Path(__file__).with_name('source-ci-completion02');OUT.mkdir(exist_ok=False)
repo='qq550723504/data-product-platform';run='39903332833'
with (OUT/'watch.stdout.txt').open('xb') as stdout,(OUT/'watch.stderr.txt').open('xb') as stderr:
    result=subprocess.run(['gh','run','watch',run,'--repo',repo,'--exit-status','--interval','30'],cwd=ROOT,stdout=stdout,stderr=stderr)
receipt={'run':int(run),'watch_exit':result.returncode,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution_authority':False}
with (OUT/'result.json').open('x',encoding='utf-8') as stream:json.dump(receipt,stream,indent=2)
print(json.dumps(receipt))
raise SystemExit(result.returncode)
