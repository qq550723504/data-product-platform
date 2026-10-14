"""Reuse #298's complete read-only oracle for the actual #299 repaired release.

Only the finite fixture label differs. Every row, membership, native provider,
phase, gate, comparison, and nonempty assertion is retained verbatim.
"""
import hashlib, json
from pathlib import Path
root=Path(__file__).resolve().parent
original=root.parents[1]/'examples/enterprise-activity/postgres-ingestion/closure_verify.py'
source=original.read_text(encoding='utf-8')
assert source.count('"release-outage"')==5
adapted=source.replace('"release-outage"','"release-repaired-outage"')
(root/'closure-oracle-binding.json').write_text(json.dumps({'originalSha256':hashlib.sha256(original.read_bytes()).hexdigest(),'adaptedSourceSha256':hashlib.sha256(adapted.encode()).hexdigest(),'onlyChange':'finite fixture label release-outage to actual release-repaired-outage','coreMutations':0,'assertionsRelaxed':0},indent=2))
exec(compile(adapted,str(original),'exec'),{'__name__':'__main__'})
