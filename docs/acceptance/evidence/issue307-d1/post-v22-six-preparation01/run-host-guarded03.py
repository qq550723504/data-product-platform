import sys,subprocess,runpy
from pathlib import Path
sys.dont_write_bytecode=True
R=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(R/'examples/lifecycle-d1'))
def refuse(*a,**k):raise RuntimeError('independent offline harness refuses all unmodeled external subprocesses')
class RefusedPopen(subprocess.Popen):
    def __init__(self,*a,**k):refuse(*a,**k)
subprocess.run=refuse;subprocess.Popen=RefusedPopen
sys.argv=[str(R/'examples/lifecycle-d1/test_six_candidate_runtime.py'),'-v']
runpy.run_path(sys.argv[0],run_name='__main__')
