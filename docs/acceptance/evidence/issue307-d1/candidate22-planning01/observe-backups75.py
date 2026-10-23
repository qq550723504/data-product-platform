"""Read retained 66 and current 9 backup originals; no native I/O."""
import datetime as dt,hashlib,json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[5];BASE=ROOT/'docs/acceptance/evidence/issue307-d1';PRIVATE=Path('C:/Users/Henry/.codex/fixture-private');HERE=Path(__file__).parent
load=lambda p:json.loads(p.read_bytes());sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def need(ok,msg):
 if not ok:raise ValueError(msg)
def main():
 prior=load(BASE/'candidate21-planning01/retained-backups66-readback01.json');old=prior['old']+prior['new'];need(len(old)==66,'old backup membership differs')
 for row in old:
  path=PRIVATE/row['private_relative_path'];need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'old backup changed')
 c=load(BASE/'remaining-candidate22/candidate.json');new=[]
 for g in c['Groups']:
  for file,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
   path=PRIVATE/'remaining-candidate22'/g['Group']/'checkpoint'/file;need(sha(path)==g[key],'new backup changed');new.append(dict(private_relative_path=path.relative_to(PRIVATE).as_posix(),sha256=sha(path),bytes=path.stat().st_size))
 need(len(new)==9,'new backup membership differs')
 result=dict(status='PASS_ALL_75_ACTUAL_BACKUP_FILES_RETAINED',at=dt.datetime.now(dt.timezone.utc).isoformat(),execution=c['Execution'],old_complete_checkpoint_files=63,old_failed_partial_files=3,old_total=66,new_total=9,total=75,old=old,new=new,complete_entry_old_backups_retained_field=66,provider_io=0,native_delete=0)
 with (HERE/'retained-backups75-readback01.json').open('xb') as f:f.write((json.dumps(result,indent=2)+'\n').encode())
 print(json.dumps(dict(status=result['status'],old=66,new=9,native_delete=0)))
if __name__=='__main__':main()
