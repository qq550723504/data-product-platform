"""Read exact prior57 plus current9 backup bytes; no runtime I/O."""
import datetime as dt,hashlib,json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[5];BASE=ROOT/'docs/acceptance/evidence/issue307-d1';PRIVATE=Path('C:/Users/Henry/.codex/fixture-private');HERE=Path(__file__).parent
load=lambda p:json.loads(p.read_bytes());sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def need(ok,msg):
 if not ok:raise ValueError(msg)
def main():
 previous=load(BASE/'candidate19-planning01/retained-backups57-readback01.json');old=previous['old']+previous['new'];need(len(old)==57,'prior backup set differs')
 for row in old:
  path=PRIVATE/row['private_relative_path'];need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'old backup drift')
 candidate=load(BASE/'remaining-candidate20/candidate.json');new=[]
 for g in candidate['Groups']:
  for file,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
   path=PRIVATE/'remaining-candidate20'/g['Group']/'checkpoint'/file;need(sha(path)==g[key],'current backup drift');new.append(dict(private_relative_path=path.relative_to(PRIVATE).as_posix(),sha256=sha(path),bytes=path.stat().st_size))
 need(len(new)==9,'current backup set differs')
 result=dict(status='PASS_ALL_66_ACTUAL_BACKUP_FILES_RETAINED',at=dt.datetime.now(dt.timezone.utc).isoformat(),execution=candidate['Execution'],old_complete_checkpoint_files=54,old_failed_partial_files=3,old_total=57,new_total=9,total=66,old=old,new=new,complete_entry_old_backups_retained_field=48,provider_io=0,native_delete=0)
 with (HERE/'retained-backups66-readback01.json').open('x',encoding='utf-8',newline='\n') as f:json.dump(result,f,indent=2);f.write('\n')
 print(json.dumps(dict(status=result['status'],old=57,new=9,native_delete=0)))
if __name__=='__main__':main()
