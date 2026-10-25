"""Summarize actual logs; keep bulky raw outputs in the private archive."""
import hashlib,json,re
from pathlib import Path
out=Path(__file__).parent;private=Path('C:/Users/Henry/.codex/fixture-private/merge-closeout01')
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
platforms={}
for name,folder in [('Windows',private/'windows-output01'),('Linux',private/'linux-output01/collection01')]:
    value=json.loads((folder/'result.json').read_bytes());assert value['status']=='PASS' and value['python'].startswith('3.13.16 ')
    assert len(value['collection'])==26 and len(value['results'])==52
    modes={}
    for mode in ('normal','O'):
        rows=[v for v in value['results'] if v['name'].endswith('-'+mode)];methods=skips=0;suites=[]
        for row in rows:
            assert row['returncode']==0 and not row['denied']
            stderr=(folder/(row['name']+'.stderr')).read_text(encoding='utf-8')
            count=int(re.search(r'Ran (\d+) tests? in ',stderr)[1]);methods+=count
            skipped=re.search(r'OK \(skipped=(\d+)\)',stderr);skips+=int(skipped[1]) if skipped else 0
            suites.append(dict(name=row['name'],methods=count,skipped=int(skipped[1]) if skipped else 0))
        assert methods==192
        modes[mode]=dict(commands=len(rows),methods=methods,skipped=skips,suites=suites)
    platforms[name]=dict(status='PASS',python=value['python'],platform=value['platform'],modes=modes,commands=52,denied_host_reads=0)
closed=json.loads((private/'linux-helper-closed01.json').read_bytes());assert closed['status']=='REMOVED'
save(out/'regression-summary01.json',dict(status='PASS_REAL_PY31316_WINDOWS_LINUX_NONDELETE',platforms=platforms,
    linux_test_helper=closed,retained_source_start=0,provider_calls=0,protection_write=0,business_delete=0,
    no_new_business_candidate_execution_window=True,raw_logs_root=str(private),new_private_evidence_not_duplicated_into_PR=True))
print(json.dumps(dict(status='PASS',commands_each_OS=52,methods_each_mode=192,linux_helper_removed=True)))
