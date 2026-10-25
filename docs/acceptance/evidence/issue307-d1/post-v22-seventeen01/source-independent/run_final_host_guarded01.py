import runpy,subprocess,sys
from unittest.mock import patch
from pathlib import Path
ROOT=Path(__file__).resolve().parents[6]
path=ROOT/'examples/lifecycle-d1/test_post_v22_seventeen.py'
sys.argv=[str(path),'-v']
# Any command not explicitly replaced by the composition is an immediate test
# failure. Popen denies indirect/check_call paths too; no Docker can escape.
with patch.object(subprocess,'run',side_effect=AssertionError('GLOBAL_UNMODELED_SUBPROCESS_DENIED')),patch.object(subprocess,'Popen',side_effect=AssertionError('GLOBAL_UNMODELED_POPEN_DENIED')):
 runpy.run_path(str(path),run_name='__main__')
