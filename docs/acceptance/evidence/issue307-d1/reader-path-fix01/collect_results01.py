"""Collect existing isolated regression facts; never rerun or touch sources."""
import hashlib,json,re,subprocess
from pathlib import Path

root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
private=Path('C:/Users/Henry/.codex/fixture-private/reader-path-fix01')
def sha(raw):return hashlib.sha256(raw).hexdigest()
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,ensure_ascii=False,indent=2)
def copy(source,target):
    with target.open('xb') as f:f.write(source.read_bytes())
summary={}
for platform,source in [('Windows',private/'windows313-output01'),('Linux',private/'linux313-output01/collection01')]:
    target=out/(platform.lower()+'313-collection01');target.mkdir(exist_ok=False)
    for path in sorted(source.iterdir()):
        if path.is_file():copy(path,target/path.name)
    result=json.loads((target/'result.json').read_bytes())
    assert result['status']=='PASS' and len(result['collection'])==26 and len(result['results'])==52
    assert result['python'].startswith('3.13.16 ')
    modes={}
    for mode in ('normal','O'):
        rows=[x for x in result['results'] if x['name'].endswith('-'+mode)]
        count=skipped=0;skip_suites=[]
        for row in rows:
            assert row['returncode']==0 and not row['denied']
            stderr=(target/(row['name']+'.stderr')).read_text(encoding='utf-8')
            match=re.search(r'Ran (\d+) tests? in ',stderr);assert match,row['name']
            count+=int(match[1]);skip=re.search(r'OK \(skipped=(\d+)\)',stderr)
            if skip:skipped+=int(skip[1]);skip_suites.append(row['name'])
        assert len(rows)==26 and count==185,(platform,mode,count)
        modes[mode]=dict(commands=len(rows),methods=count,skipped=skipped,skip_suites=skip_suites)
    summary[platform]=dict(status='PASS',python=result['python'],commands=52,modes=modes,
                           forbidden_host_reads=0,retained_source_start=0,business_provider_io=0)
copy(private/'python313-windows-runtime01.json',out/'windows313-runtime01.json')
copy(private/'python-image01/Dockerfile',out/'linux313-Dockerfile')
raw=subprocess.check_output(['docker','inspect','dpp307-reader-path-python313-01'])
item=json.loads(raw)[0]
assert item['State']['Running'] is False and item['State']['Pid']==0 and item['State']['ExitCode']==0
assert item['HostConfig']['NetworkMode']=='none' and item['Config']['User']=='10001:10001'
assert not item['NetworkSettings']['Ports'] and len(item['Mounts'])==2
assert next(m for m in item['Mounts'] if m['Destination']=='/work')['RW'] is False
assert {m['Destination'] for m in item['Mounts']}=={'/work','/out'}
save(out/'linux313-container-closure01.json',dict(cid=item['Id'],image=item['Image'],state=item['State'],
     network_mode=item['HostConfig']['NetworkMode'],user=item['Config']['User'],mounts=item['Mounts'],
     cap_drop=item['HostConfig']['CapDrop'],security_opt=item['HostConfig']['SecurityOpt'],
     ports=item['NetworkSettings']['Ports'],retained_mounts=0))
save(out/'regression-summary01.json',dict(status='PASS_REAL_CPYTHON_31316_WINDOWS_LINUX_OFFLINE',platforms=summary,
     boundaries='Tests double provider/Docker business operations. Python open audit is test-only, not an OS sandbox; Linux additionally uses network-none read-only isolated snapshot.',
     composition_methods_per_mode=12,windows_composition_skips=0,new_head_PR_CI='NOT_RUN'))
save(out/'original-failure-binding01.json',dict(original_published_head='16ec447f2c60ddf08a514207963f478bc70c7e09',
     original_test_raw_sha256='c24267a81183ca47d29c0af9a487cb3b7f809b3782db90a4537c7229e61c9261',
     original_PR_run=40099260254,original_job=120887885664,python='3.13.16',
     cause='Fixture derived and signed reader path strings from an unnormalized root; Composition canonicalized control and correctly rejected exact path mismatch.',
     verified_reproduction=['original source canonical TEMP PASS','same physical Windows folder uppercase alias REJECTED','same physical Windows folder real 8.3 alias REJECTED'],
     uncertainty='The original CI runner did not print both path spellings; its exact alias spelling is not established. Python version alone is not the root cause.',
     fix='Reuse indexes.unlinked_absolute before any fixture IO/path derivation/signing; no production gate relaxation.',
     runtime_source_unchanged='0446fde5985b51de7da5863b45df3f1b47b6167c'))
print(json.dumps(summary))
