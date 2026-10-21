"""Append repair evidence and byte-preserve the original failed execution."""
import hashlib,json,pathlib,shutil,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[5]
OUT=pathlib.Path(__file__).parent
OLD=pathlib.Path('C:/Users/Henry/.codex/worktrees/d1-c18/data-product-platform')
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def save(name,obj):
    with (OUT/name).open('x',encoding='utf-8',newline='\n') as f:json.dump(obj,f,ensure_ascii=False,indent=2);f.write('\n')

files=subprocess.check_output(['git','-c','core.longpaths=true','ls-files','--others','--exclude-standard'],cwd=OLD).decode('utf-8').splitlines()
members=[]
for rel in files:
    if not rel.startswith('docs/acceptance/'):raise RuntimeError('unexpected original untracked path')
    source=OLD/rel;target=ROOT/rel
    if target.exists():raise RuntimeError('copy would overwrite '+rel)
    target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,target)
    if sha(source)!=sha(target):raise RuntimeError('copy differs')
    members.append({'path':rel,'sha256':sha(source)})
save('original-c18-byte-preservation.json',{'status':'PASS_EXACT_APPEND_ONLY_COPY','members':members,'original_checkout_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=OLD).decode().strip(),'source_checkout_modified':False})
p=subprocess.run(['go','mod','download','-json','github.com/minio/minio@v0.0.0-20251015172955-9e49d5e7a648'],cwd=ROOT/'apps/platform',stdout=subprocess.PIPE,stderr=subprocess.PIPE)
(OUT/'actual-native-module.json').write_bytes(p.stdout)
if p.returncode:raise RuntimeError('actual module metadata unavailable')
cmd=['go','test','./internal/lifecycle/...','./internal/platform/storage','./cmd/lifecycle-fixture-remaining','./cmd/lifecycle-fixture-reader','./cmd/lifecycle-fixture-remaining-iam']
p=subprocess.run(cmd,cwd=ROOT/'apps/platform',stdout=subprocess.PIPE,stderr=subprocess.PIPE)
(OUT/'go-regression.stdout.txt').write_bytes(p.stdout);(OUT/'go-regression.stderr.txt').write_bytes(p.stderr)
save('go-regression.json',{'command':cmd,'exit_code':p.returncode,'unconfigured_live_postgres':'NOT_RUN','native03_after_build_change':'Only gofmt -w on send_return.go and wire_native_test.go; no semantic edit after native03. Original compiled native receipts retained, final formatted source rerun by Go tests.','source_changes_during_native03':False})
if p.returncode:raise RuntimeError('Go regression failed')
