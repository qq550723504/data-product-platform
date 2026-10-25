import sys,subprocess,runpy
from pathlib import Path
sys.dont_write_bytecode=True
R=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(R/'examples/lifecycle-d1'))
def refuse(*a,**k):raise RuntimeError('independent offline harness refuses all external subprocesses')
subprocess.run=refuse;subprocess.Popen=refuse
sys.argv=[str(R/'examples/lifecycle-d1/test_remaining_sequence.py')]
runpy.run_path(sys.argv[0],run_name='__main__')
