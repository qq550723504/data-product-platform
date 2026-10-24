import sys,subprocess,runpy
from pathlib import Path
sys.dont_write_bytecode=True
R=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(R/'examples/lifecycle-d1'))
name=sys.argv[1]
if name not in {'test_freeze_six_candidate.py','test_window_six_support.py','test_verify_six_native.py'}:raise ValueError('unbound offline test')
def refuse(*a,**k):raise RuntimeError('independent offline harness refuses unmodeled external subprocess')
class RefusedPopen(subprocess.Popen):
    def __init__(self,*a,**k):refuse(*a,**k)
subprocess.run=refuse;subprocess.Popen=RefusedPopen
sys.argv=[str(R/'examples/lifecycle-d1'/name),'-v']
runpy.run_path(sys.argv[0],run_name='__main__')
