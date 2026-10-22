"""Finite candidate21 host maintenance, reusing the reviewed containment tools.

No enable, sender, deletion or scheduling operation is exposed. Archive/revoke/
H1 proof/seal retain the prior contracts; current identities, mounts and six
limits are bound explicitly. The historical support file is never modified.
"""
import argparse
import base64
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import uuid
import execution_freeze

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('six_complete_for_host',Path(__file__).with_name('verify-six-native.py'))
v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
r,p=v.r,v.p
old_path=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-window-review06/composition/window_support.py'
if hashlib.sha256(old_path.read_bytes()).hexdigest()!='ad4a7179efb96a02ef23401f1fbe5d489452f5cd923b09c7aae6fba12944738f':raise ValueError('reviewed containment dependency differs')
spec=importlib.util.spec_from_file_location('six_retained_host_primitives',old_path)
base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)
need,load,sha,exclusive=base.need,base.load,base.sha,base.exclusive
PACKET=v.PACKET;PRIVATE=r.CONTROL;CONTROL=PRIVATE/'authorized-execution01'


def configure():
    anchor=load(PRIVATE/'host-anchor.json')
    need(anchor['execution']==p.EXECUTION and anchor['profile']==p.PROFILE,'new host anchor scope differs')
    base.ROOT=ROOT;base.PACKET=PACKET;base.PACKET_RELATIVE=PACKET.relative_to(ROOT).as_posix()+'/'
    base.AUTHORIZATION_PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/delete-confirmed-v31-execution01/human-authorization.json'
    base.PRIVATE=PRIVATE;base.CONTROL=CONTROL;base.EXECUTION=p.EXECUTION
    base.CANDIDATE=anchor['candidate_sha256'];base.BINDINGS=anchor['bindings_sha256'];base.TEMPLATE=anchor['template_sha256'];base.PACKET_ANCHOR=anchor['packet_commit']
    base.START=p.window.instant(p.W['NotBefore']);base.END=p.window.instant(p.W['NotAfter'])
    return anchor


class Host(base.Host):
    def __init__(self,containment=False):
        self.anchor=configure();super().__init__(containment=containment)

    def known_users(self,group):
        # Frozen exact names, not a prefix inference or an identity omitted
        # because it belonged to a previous execution.
        iam=load(PRIVATE/group['Group']/'root-disable-all/iam.json')
        need(sha(PRIVATE/group['Group']/'root-disable-all/iam.json')==group['IAMSHA256'],'known IAM set drift')
        return set(iam['disabled_identities'])

    def stop(self,ident):
        # A failed inspect must not veto the fixed known CID's stop attempt.
        # The final physical readback alone decides whether closure is proven.
        need(len(ident)==64 and all(c in '0123456789abcdef' for c in ident),'fixed full CID required for containment')
        initial_failed=False;stop_failed=False
        try:running=self.inspect(ident)['State']['Running']
        except Exception:running=True;initial_failed=True
        if running:
            try:self.docker('stop','--time','15',ident,timeout=25)
            except Exception:stop_failed=True
        try:confirmed=r.closed(self.inspect(ident))
        except Exception:confirmed=False
        need(confirmed,'fixed owner PID0/no-restart closure unconfirmed after stop attempt')
        return {'id':ident,'stopped_pid_zero':True,'initial_inspect_failed':initial_failed,'stop_failed':stop_failed}

    def check(self):
        need(base.approved_packet('candidate.json',base.CANDIDATE)==self.candidate and base.approved_packet('bindings.json',base.BINDINGS)==self.bindings,'approved containment snapshot unavailable or differs')
        auth=load(base.AUTHORIZATION_PUBLIC)
        branch_gate=execution_freeze.authorized_gate(ROOT,PRIVATE,PACKET,self.candidate)
        expected=dict(execution=p.EXECUTION,candidate_sha256=base.CANDIDATE,bindings_sha256=base.BINDINGS,referenced_template_sha256=base.TEMPLATE,
                      not_before=p.W['NotBefore'],not_after=p.W['NotAfter'],max_per_target=1,max_total=6,survivors=11,unknown_resend=False,cleanup=0,hold_release=0,bypass=0,old_authorization_reused=False)
        need(all(auth.get(k)==value for k,value in expected.items()),'new six-target authorization scope/window differs')
        for field,key in [('source','Source'),('source_freeze_sha256','SourceFreezeSHA256'),('reader_binary_sha256','ReaderBinarySHA256'),('sender_wire_recovery_binary_sha256','SenderBinarySHA256'),('owner_binary_sha256','OwnerBinarySHA256'),('target_digest','TargetDigest')]:need(auth[field]==self.candidate[key],'authorization runtime binding differs')
        need(sha(CONTROL/'user-message-verbatim.txt')==auth['user_message_sha256'] and (CONTROL/'user-message-verbatim.txt').read_bytes().decode('utf-8')==auth['user_message_verbatim']
             and sha(CONTROL/'authorization-envelope.txt')==auth['authorization_envelope_sha256'] and (CONTROL/'approved-referenced-template.txt').read_bytes()==(PACKET/'confirmation-template.txt').read_bytes(),'actual human text/envelope/reference differs')
        references=auth['approved_reference_artifacts'];need(references and len({x['path'] for x in references})==len(references),'approved reference set incomplete')
        for artifact in references:
            path=(ROOT/artifact['path']).resolve();need(path.is_relative_to(ROOT) and sha(path)==artifact['sha256'],'approved reference byte drift')
        gate=v.sealed_gate(PACKET)
        return {'status':'PASS_PREWINDOW_CLOSED','execution':p.EXECUTION,'host_gate':gate,'branch_gate':branch_gate,'containment_anchor':base.PACKET_ANCHOR,'source_provider_io':0,'native_delete':0}

    def disable_all(self,group):
        need(self.inspect(group['Native'])['State']['Running'],'native stopped; fresh identity readback unavailable')
        item,output=self.root_owner(group,'disable-all');need(base.closed(item),'disable owner active')
        archive={'complete':True}
        try:archive['receipt']=base.rotate(output)
        except Exception as error:archive={'complete':False,'error_type':type(error).__name__}
        try:stale=(output/'status-changes.json').exists()
        except OSError:stale=True
        self.docker('start',item['Id'])
        code=int(self.docker('wait',item['Id'],timeout=40).strip());readback=False
        if not stale:
            try:
                changes=load(output/'status-changes.json')
                readback=(changes['action']=='disable-all' and not changes['unconfirmed'] and set(changes['disabled_readback'])==self.known_users(group)
                          and set(changes['disabled_readback'].values())=={'disabled'} and base.closed(self.inspect(item['Id'])))
            except Exception:pass
        enablers=base.all_steps([('enabler:'+action,lambda a=action:base.closed(self.inspect(self.binding(group)['root_owners'][a]['container']))) for action in ('enable-read','enable-send')])
        closed=all(x['ok'] and x['result'] is True for x in enablers)
        return {'identity_disable_confirmed':readback and closed,'last_readback_all_disabled':readback,'enablers_closed':closed,'enablers':enablers,'archive':archive,'stale_summary_rejected':stale,
                'policy_validation_passed':code==0,'helper_exit':code,'status_changes_sha256':sha(output/'status-changes.json') if readback else None}

    def seal_gateway(self,group):
        need(base.closed(self.inspect(group['Gateway'])),'stop gateway before seal')
        folder=PRIVATE/group['Group'];mount=next(m for m in self.inspect(group['Gateway'])['Mounts'] if m['Destination']=='/etc/caddy/Caddyfile')
        path=p.p1.hostpath(mount['Source'])
        need(not mount['RW'] and path==(folder/'gateway.Caddyfile').resolve(),'actual gateway config path differs')
        golden=folder/'gateway-sealed.Caddyfile';need(sha(golden)==group['SealedGatewaySHA256'],'sealed gateway golden drift')
        archive=CONTROL/'gateway-history';archive.mkdir(parents=True,exist_ok=True)
        r.write_raw(archive/(group['Group']+'-'+uuid.uuid4().hex+'.private'),path.read_bytes())
        path.write_bytes(golden.read_bytes());need(sha(path)==group['SealedGatewaySHA256'],'gateway seal readback differs')
        return {'gateway_stopped_and_sealed':True}

    def h1_proof(self):
        execution_freeze.authorized_gate(ROOT,PRIVATE,PACKET,self.candidate)
        at=base.now();base.maintenance_time(at,signing=True)
        group=self.group('late-hold');folder=PRIVATE/'late-hold';cfg=load(folder/'case-close-H1/remaining.json')
        need(sha(folder/'case-close-H1/remaining.json')==group['CaseConfigSHA256']['close-H1'] and cfg['ClosureOwners']=={group['CaseHelpers']['late']:self.candidate['ReaderBinarySHA256']}
             and cfg['ClosurePublicKey']==self.candidate['PublicKey'],'H1 owner/key/config differs')
        owner=self.inspect(group['CaseHelpers']['late']);need(base.closed(owner) and owner['State']['ExitCode']==0,'H1 owner not successfully closed')
        r.unprivileged(owner,'/run/reader',['-mode','remaining-late'],group['Gateway'])
        r.mounts_exact(owner,{'/run/private':('bind',folder/'case-late',False),'/run/reader':('bind',PRIVATE/'reader',False),'/run/output':('bind',folder/'outputs/case-late',True)})
        revocation=CONTROL/'revocations/late-hold/late';proof=load(revocation/'revocation.json');raw=(revocation/'original.private').read_bytes()
        need(base.digest(raw)==proof['before_sha256']==group['CaseConfigSHA256']['late'],'H1 original profile not retained')
        disabled=json.loads(raw);disabled['Base']['Enabled']=False
        need(load(folder/'case-late/remaining.json')==disabled and sha(folder/'case-late/remaining.json')==proof['disabled_sha256'],'H1 profile not revoked by exactly one field')
        need(sha(PRIVATE/'reader')==self.candidate['ReaderBinarySHA256'],'H1 reader binary drift')
        for stage,cid in group['Helpers'].items():need(base.closed(self.inspect(cid)) and load(folder/stage/'permission.json')['Enabled'] is False,'H1 operator not closed')
        for action,root in self.binding(group)['root_owners'].items():need(base.closed(self.inspect(root['container'])) and load(folder/('root-'+action)/'permission.json')['Enabled'] is False,'H1 root enabler not closed')
        root,output=self.root_owner(group,'check-disabled');base.fresh_finished(root,at);result=load(output/'result.json')
        need(result['action']=='check-disabled' and result['group']=='late-hold' and result['exact_policy_set_verified'] is True and result['before']=={name:'disabled' for name in self.known_users(group)},'H1 fresh full disabled IAM differs')
        gateway=self.inspect(group['Gateway']);need(base.closed(gateway),'H1 gateway still active')
        mount=next(m for m in gateway['Mounts'] if m['Destination']=='/etc/caddy/Caddyfile');need(not mount['RW'] and sha(p.p1.hostpath(mount['Source']))==group['SealedGatewaySHA256'],'H1 gateway not sealed')
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
        key=Ed25519PrivateKey.from_private_bytes((PRIVATE/'topology-key.private').read_bytes());need(key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex()==self.candidate['PublicKey'],'H1 host private key differs')
        body=dict(Execution=p.EXECUTION,Nonce=str(uuid.uuid4()),Group='late-hold',CandidateSHA256=cfg['ClosureCandidateSHA256'],Owners=cfg['ClosureOwners'],At=at.isoformat(),StoppedPIDZero=True,NoRestart=True,ConfigDisabled=True,DeleteIdentityDisabled=True,GatewaySealed=True)
        encoded=base.compact(body);exclusive(folder/'case-close-H1/closed-H1.json',{'Body':base64.b64encode(encoded).decode(),'Signature':base64.b64encode(key.sign(encoded)).decode()})
        receipt={'at':at.isoformat(),'execution':p.EXECUTION,'proof_sha256':sha(folder/'case-close-H1/closed-H1.json'),'root_check_finished_at':root['State']['FinishedAt'],'root_result_sha256':sha(output/'result.json'),'revocation_sha256':sha(revocation/'revocation.json'),'native_delete':0}
        exclusive(CONTROL/'H1-proof.json',receipt);return receipt


def main():
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=('check','archive-root','revoke-case','h1-proof','seal'));parser.add_argument('--group',choices=p.GROUPS);parser.add_argument('--stage');args=parser.parse_args()
    host=Host(containment=args.action=='seal')
    if args.action=='check':out=host.check()
    elif args.action=='seal':out=host.seal()
    elif args.action=='h1-proof':out=host.h1_proof()
    else:
        need(args.group is not None and args.stage is not None,'fixed group/stage required')
        group=host.group(args.group)
        out=host.archive_root(group,args.stage) if args.action=='archive-root' else host.revoke_case(group,args.stage)
    print(json.dumps(out,ensure_ascii=False))


if __name__=='__main__':main()
