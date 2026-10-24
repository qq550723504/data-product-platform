"""Reuse the accepted isolated Linux probe; preserve old probe and volumes."""
from pathlib import Path
root=Path(__file__).resolve().parents[5]
source=root/'docs/acceptance/evidence/issue307-d1/retention-runtime-fix03/probe_linux03.py'
raw=source.read_text(encoding='utf-8')
raw=raw.replace('retention-runtime-fix03','retention-composition-v2-acceptance01')
raw=raw.replace("['docker','create','--name'","['docker','create','--pull','never','--name'")
path=Path(__file__).with_name('probe_linux04.py')
with path.open('x',encoding='utf-8',newline='\n') as stream:stream.write(raw)
