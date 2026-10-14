"""Real Docker regression for isolated builds; retains every trial resource.

Runs only synthetic raw extraction and native image inspection. No Core,
provider object deletion, cleanup, or existing container start is performed.
"""
import argparse,hashlib,ipaddress,json,subprocess,uuid
from pathlib import Path
import smoke,storage_smoke
from retained_runtime import DurableStorage,private_directory
from trial_images import TrialImages

SHARED_IMAGE_ALIASES = ('dpp-pgi-hop-jdbc:42.7.14', 'dpp-pgi-minio:2025-10-15')

def shared_image_snapshot(command):
    snapshot = {}
    for ref in SHARED_IMAGE_ALIASES:
        result = command(['docker', 'image', 'inspect', ref], check=False)
        if result.returncode:
            missing = ('Error response from daemon: No such image: ' + ref).encode()
            smoke.require(result.returncode == 1 and result.stderr.strip() == missing,
                          'cannot inspect shared image alias: ' + ref)
            snapshot[ref] = None
        else:
            snapshot[ref] = json.loads(result.stdout)[0]['Id']
    return snapshot

def run(output,cache,subnet):
    output.mkdir(parents=True,exist_ok=False)
    original=smoke.command
    proposed=ipaddress.ip_network(subnet)
    ids=original(['docker','network','ls','-q']).stdout.decode().split()
    for network in json.loads(original(['docker','network','inspect',*ids]).stdout):
        for cfg in network.get('IPAM',{}).get('Config') or []:
            if cfg.get('Subnet'):
                allocated=ipaddress.ip_network(cfg['Subnet'])
                assert proposed.version!=allocated.version or not proposed.overlaps(allocated)
    def command(args,*pos,**kw):
        if args[:3]==['docker','network','create']:args=args[:3]+['--subnet',subnet]+args[3:]
        return original(args,*pos,**kw)
    def image(ref):return json.loads(original(['docker','image','inspect',ref]).stdout)[0]['Id']
    before=shared_image_snapshot(original)
    native=[]
    for index in range(2):
        owner=TrialImages(output,str(uuid.uuid4()),True,original,stem='minio-image-ownership-'+str(index))
        tag=owner.tag('minio')
        binary,identity=storage_smoke.prepare_binary(cache,tag,owner=owner)
        assert identity==image(tag)
        name='dpp-image-pin-'+uuid.uuid4().hex
        original(['docker','create','--name',name,'--label','dpp.image-isolation=true','--entrypoint','/usr/local/bin/minio',identity,'--version'])
        configured=json.loads(original(['docker','inspect',name]).stdout)[0]
        assert configured['Image']==identity and configured['Config']['Image']==identity
        version=original(['docker','start','--attach',name]).stdout.decode()
        native.append({'tag':tag,'imageId':identity,'container':name,'binarySha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'version':version.strip()})
    assert native[0]['tag']!=native[1]['tag']
    assert image(native[0]['imageId'])==native[0]['imageId'],'later build lost earlier image'
    private=private_directory('dpp-image-isolation-')
    durable=DurableStorage(output,private,smoke.REPO/'examples/lifecycle-d1/durable-storage.ps1')
    trial=smoke.Trial(output,retain=True,durable_start=durable.start)
    try:
        smoke.command=command
        trial.setup()
        assert trial.engine_image==image('dpp-pgi-hop-jdbc:trial-'+trial.run_id.replace('-',''))
        configured=json.loads(original(['docker','inspect',trial.hop]).stdout)[0]
        assert configured['Image']==trial.engine_image and configured['Config']['Image']==trial.engine_image
        trial.fidelity()
        after=shared_image_snapshot(original)
        assert before==after,'trial rewrote shared image tags'
        report={'status':'PASS','sharedBefore':before,'sharedAfter':after,'minio':native,
            'hopImage':trial.engine_image,'hopContainer':trial.hop,'sourceContainer':trial.pg,'trialRunId':trial.run_id,
            'sourcePrivate':str(trial.private),'nativeExtraction':trial.report.get('checks',trial.report),'cleanup':0,'providerDelete':0,
            'checks':['two fresh MinIO tags retain the first immutable image','container Config.Image pins resolved ID','Hop uses unique run tag and immutable ID','real Hop 6/10/14 extraction matches original strings and repeated byte hashes','shared legacy aliases unchanged']}
        (output/'image-isolation.json').write_text(json.dumps(report,indent=2),encoding='utf-8')
        print(json.dumps({'status':'PASS','checks':len(report['checks']),'runId':trial.run_id,'minioImages':[v['imageId'] for v in native],'hopImage':trial.engine_image}),flush=True)
    finally:
        smoke.command=original
        if trial.proxy:trial.proxy.shutdown();trial.proxy.server_close()
        # No cleanup: runtime namespaces, providers, volumes, credentials and files survive.

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);p.add_argument('--cache',type=Path,required=True);p.add_argument('--subnet',default='10.249.202.0/24')
    a=p.parse_args();run(a.output.resolve(),a.cache.resolve(),a.subnet)
