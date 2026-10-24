"""Reuse the known-credential comparator, extending only the final evidence set."""
from pathlib import Path

HERE = Path(__file__).parent
source_path = HERE.parent / 'candidate21-planning01/scan-publication-secrets01.py'
source = source_path.read_text(encoding='utf-8')
old = "'candidate21-artifacts-review01','candidate21-planning01')"
if source.count(old) != 1:
    raise ValueError('Original frozen scanner source differs')
source = source.replace(old, "'candidate21-artifacts-review01','candidate21-planning01','candidate21-publication01')")
source = source.replace("HERE/'publication-secret-scan01.json'", "HERE/'publication-secret-scan02.json'")
exec(compile(source, str(source_path), 'exec'), {'__file__': str(Path(__file__).resolve()), '__name__': '__main__'})
