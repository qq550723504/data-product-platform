"""Summarize actual exact receipts and current disk state, never a remover."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
ARCHIVE=Path('C:/Users/Henry/.codex/fixture-private/resource-retirement01/capture06')
PRIOR=ARCHIVE.parent/'budget-forensics01'
SOURCE=ARCHIVE.parent/'capture05'
PUBLIC=Path(__file__).parent

def main():
    container_result=r.read(ARCHIVE/'remove-containers01/result.json')
    volume_result=r.read(ARCHIVE/'remove-volumes01/result.json')
    r.need(container_result['status']=='PASS_RETIRED_HISTORY' and container_result['removed']==986
           and volume_result['status']=='PASS_RETIRED_CONTAINER_AND_TEST_VOLUME_HISTORY' and volume_result['removed_volumes']==106,
           'do not publish an unfinished removal as complete')
    parent=r.read(ARCHIVE/'plan.json');volume_plan=r.read(ARCHIVE/'volume-plan.json')
    journal=ARCHIVE/'final-readback-journal';journal.mkdir();docker=r.Docker(journal)
    before=r.read(PRIOR/'all-containers.json');ids=docker.inventory();rows=docker.inspect(sorted(ids))
    removed=set(parent['retired_historical'])
    r.need(not removed&ids and set(before)-removed<=ids,'unapproved old container disappeared')
    for cid in set(before)-removed:
        r.need(r.descriptor(rows[cid])==r.descriptor(before[cid]),'unapproved retained container descriptor changed')
    live_changes={cid:dict(before=r.state(before[cid]),after=r.state(rows[cid])) for cid in set(before)-removed if r.state(rows[cid])!=r.state(before[cid])}
    r.save(ARCHIVE/'final-containers.json',rows)
    current_volumes=docker.raw('volume','ls','-q').decode('utf8','strict').split()
    before_volume_names={entry['Name'] for entry in r.read(SOURCE/'docker-df-before.stdout')['Volumes']}
    removed_volumes=set(volume_plan['remove'])
    r.need(not removed_volumes&set(current_volumes) and set(volume_plan['preserve'])<=set(current_volumes),'final volume decision state differs')
    actual_allocated={}
    names=sorted(volume_plan['preserve'])
    for at in range(0,len(names),40):
        paths=['/mnt/docker-desktop-disk/data/docker/volumes/'+name+'/_data' for name in names[at:at+40]]
        result=subprocess.run(['wsl','-d','docker-desktop','--','du','-sk',*paths],capture_output=True,timeout=90)
        for channel in ('stdout','stderr'):r.write(ARCHIVE/('volume-du-after-%03d.'%at+channel),getattr(result,channel))
        r.need(result.returncode==0,'fresh retained allocated-volume bytes unavailable')
        for line in result.stdout.decode('utf8','strict').splitlines():
            size,path=line.split(None,1);actual_allocated[path.split('/')[-2]]=int(size)*1024
    before_allocated=r.read(SOURCE/'volume-allocated-before.json')
    r.need(actual_allocated=={name:before_allocated[name] for name in names},'retained recovery-volume allocated bytes changed')
    internal=subprocess.run(['wsl','-d','docker-desktop','--','df','-k','/mnt/docker-desktop-disk/data'],capture_output=True,timeout=30)
    r.need(internal.returncode==0,'internal filesystem observation unavailable')
    r.write(ARCHIVE/'internal-df-after.stdout',internal.stdout)
    def used(raw):return int(raw.decode('utf8').splitlines()[-1].split()[2])*1024
    indexes={}
    for directory,filename,status in ((ARCHIVE/'removals','removal-index.json','EXACT_RETIREMENT_RECEIPTS'),(ARCHIVE/'volume-removals','volume-removal-index.json','EXACT_TEST_VOLUME_RETIREMENT_RECEIPTS')):
        if not (ARCHIVE/filename).exists():
            files={path.relative_to(ARCHIVE).as_posix():dict(sha256=r.sha(path.read_bytes()),bytes=path.stat().st_size) for path in sorted(directory.rglob('*')) if path.is_file()}
            r.save(ARCHIVE/filename,dict(status=status,format='files-map-v1',count=len(files),files=files))
        index=r.read(ARCHIVE/filename)
        audit=r.audit.audit_index(ARCHIVE,dict(id=filename,index=filename,sha256=r.sha((ARCHIVE/filename).read_bytes()),format='files-map-v1',member_root='.',expected_count=index['count'],expected_status=status))
        indexes[filename]=audit
    container_rows=[]
    for cid in sorted(removed):
        outcome=ARCHIVE/'removals'/cid/'result.json'
        container_rows.append(dict(cid=cid,name=before[cid]['Name'].lstrip('/'),status=r.read(outcome)['status'],receipt_sha256=r.sha(outcome.read_bytes())))
    volume_rows=[]
    for name in sorted(volume_plan['candidate_volumes']):
        decision='remove' if name in removed_volumes else 'preserve';entry=volume_plan[decision][name]
        row=dict(name=name,decision='REMOVED' if decision=='remove' else 'PRESERVED',reason=entry['reason'],allocated_bytes_before=before_allocated[name])
        if decision=='remove':row['receipt_sha256']=r.sha((ARCHIVE/'volume-removals'/r.sha(name.encode())/'result.json').read_bytes())
        volume_rows.append(row)
    r.save(PUBLIC/'removed-containers01.json',dict(count=len(container_rows),containers=container_rows))
    r.save(PUBLIC/'volume-decisions01.json',dict(count=len(volume_rows),volumes=volume_rows))
    space_before=r.read(ARCHIVE/'space-boundary-before.json');space_after=dict(at=r.now(),c_drive=shutil.disk_usage('C:/')._asdict(),vhd_bytes=Path('C:/Users/Henry/AppData/Local/Docker/wsl/disk/docker_data.vhdx').stat().st_size)
    r.save(ARCHIVE/'space-boundary-after.json',space_after)
    internal_before=used((SOURCE/'internal-df-before.stdout').read_bytes());internal_after=used(internal.stdout)
    result=dict(status='PASS_FINISHED_EXACT_REMOVALS_PENDING_CATALOG_CHECK',at=r.now(),containers_removed=len(removed),volumes_removed=len(removed_volumes),
        volumes_preserved_in_candidate_scope=len(volume_plan['preserve']),budget_volumes_preserved=len(parent['budgets']),
        containers_before=len(before),containers_after=len(rows),volumes_before=len(before_volume_names),volumes_after=len(current_volumes),other_retained_container_state_changes=live_changes,
        unrelated_volume_missing=sorted(before_volume_names-set(current_volumes)-removed_volumes),new_unrelated_volumes=sorted(set(current_volumes)-before_volume_names),
        removed_volume_allocated_bytes=sum(before_allocated[name] for name in removed_volumes),removed_container_logical_writable_bytes=r.read(ARCHIVE/'container-sizes-before/result.json')['total_bytes'],
        internal_filesystem_used_before=internal_before,internal_filesystem_used_after=internal_after,internal_filesystem_observed_net_freed_bytes=internal_before-internal_after,
        c_free_before=space_before['c_drive']['free'],c_free_after=space_after['c_drive']['free'],c_observed_net_free_change_bytes=space_after['c_drive']['free']-space_before['c_drive']['free'],
        offline_vhd_compaction=False,docker_stopped=False,source_started=0,provider_object_DELETE=0,retention_hold_IAM_write=0,budgets_refunded=0,pins_released=0,
        original_files_retained=container_result['original_files'],indexes=indexes,
        PR332='OPEN/Draft/unmerged; no publication by this cleanup',Issue307='OPEN')
    r.save(PUBLIC/'cleanup-physical-result01.json',result)
    print(json.dumps(result),flush=True)
if __name__=='__main__':main()
