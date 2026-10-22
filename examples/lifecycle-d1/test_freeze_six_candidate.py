"""Finite freeze composition from recorded non-delete receipts, no Docker."""
from contextlib import ExitStack
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('six_freeze_under_test',Path(__file__).with_name('freeze-six-candidate.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f)


class FreezeSixTests(unittest.TestCase):
    def test_complete_six_packet_and_confirmation_with_eleven_survivors(self):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp);control=root/'private';public=root/'preparation';packet=root/'packet';fixtures=root/'fixtures'
            control.mkdir();public.mkdir();fixtures.mkdir();(control/'offline').mkdir()
            for obj,name,value in [(f.r,'CONTROL',control),(f.r,'PUBLIC',public),(f.r,'ROOT',root),(f,'PACKET',packet),(f.p.p1,'PRIVATE',fixtures)]:stack.enter_context(patch.object(obj,name,value))
            old=f.load(f.p.p1.PACKET/'candidate.json');scopes=f.load(f.p.ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json')
            allocation={'source':'f'*40,'build_files':{},'retained':{'mode':'RECORDED_TEST_ONLY'},'owner_descriptors_sha256':'d'*64,'map_sha256':'a'*64,'binaries':{'reader':'1'*64,'operator':'2'*64,'owner':'3'*64},'groups':[],'offline_validator':'v'*64}
            for name in ('reader','operator','owner','readonly-check.exe'):(control/name).write_bytes(b'non-authoritative-test-binary')
            for old_group,scope in zip(old['Groups'],scopes):
                group=scope['Group'];project='dpp-lifecycle307-'+scope['Run'][:8];folder=control/group;folder.mkdir();out=public/group;out.mkdir()
                (folder/'checkpoint').mkdir()
                backups={}
                for name in ('core.dump','globals.sql','native.tar'):(folder/'checkpoint'/name).write_bytes((group+name).encode());backups[name]=f.sha(folder/'checkpoint'/name)
                (fixtures/project).mkdir()
                config={k:'test-only' for k in ('RootAccess','RootSecret','PostgresDSN','RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint')};config['ActorID']=old_group['Actor']
                f.save(fixtures/project/'config.json',config)
                access=f.p.identity(scope,'inspect');delete=f.p.identity(scope,'delete');f.save(folder/'new-secrets.private.json',{access:'test-inspection',delete:'test-send'})
                (folder/'gateway.Caddyfile').write_bytes(b':9000 { respond 403 }\n')
                cases={stage:('case-'+group+'-'+stage).ljust(64,'0') for stage in f.p.window.CASE_MODES[group]}
                helpers={k:v for k,v in old_group['Helpers'].items() if k not in ('P1','recover-P1')}
                heads=f.p.window.policy_heads(group)
                for stage in cases:
                    owner=folder/('case-'+stage);owner.mkdir()
                    cfg=f.p.window.case_profile({'Group':group,'Base':{'Enabled':True},'NonDelete':True},heads,'test-token','test-epoch')
                    f.save(owner/'remaining.json',cfg);(owner/'manifest.json').write_bytes(b'{}')
                for stage in helpers:(folder/stage).mkdir();f.save(folder/stage/'permission.json',{'Enabled':False})
                roots={action:('root-'+group+'-'+action).ljust(64,'0') for action in ('check-disabled','enable-read','enable-send','disable-send','disable-all')}
                for action in roots:(folder/('root-'+action)).mkdir();f.save(folder/('root-'+action)/'permission.json',{'Enabled':False})
                g={'group':group,'private':str(folder),'scope':scope,'source':{k:old_group[k] for k in ('Core','Native','Network')},'gateway':old_group['Gateway'],'restore_core':old_group['CheckpointCore'],'restore_native':old_group['CheckpointNative'],
                   'cases':cases,'helpers':helpers,'root_owners':roots,'recovery_redis':'r'*64,'volumes':{'budget':'test-'+group+'-budget'},'budget_initializer':'b'*64,'budget_initializer_script_sha256':'e'*64}
                allocation['groups'].append(g)
                f.save(out/'checkpoint.json',{'backups':backups})
                receipt=f.load(f.p.p1.PACKET/group/'readonly/native-verification.json')
                receipt['copies']=[x for x in receipt['copies'] if x['Copy']['Key']!='fixture/p1.csv']
                evidence=out/'maintenance-source/files';evidence.mkdir(parents=True)
                for name in receipt['policy_sha256']:(evidence/('native-policy-'+name+'.json')).write_bytes((f.p.p1.PACKET/group/'readonly'/('native-policy-'+name+'.json')).read_bytes())
                f.save(evidence/'six-native-verification.json',{'verification':receipt})
                f.save(out/'source-Core-after.json',{'recorded_only':True})
                (out/'expectation').mkdir();f.save(out/'expectation/expected-contract.json',{'not_actual_native':True})
            f.save(public/'allocation.json',allocation)
            (public/'test-evidence.txt').write_bytes(b'recorded')
            f.save(public/'non-delete-evidence-index.json',{'status':'SIX_NONDELETE_PREPARATION_EVIDENCE_INDEX','count':1,'files':{'test-evidence.txt':{'sha256':f.sha(public/'test-evidence.txt'),'bytes':8}}})
            f.save(public/'non-delete-terminal.json',{'status':'PASS_NONDELETE_SOURCE_RESTORE_PREPARATION','groups_completed':3,'native_delete':0,'mode':'RECORDED_TEST_ONLY','evidence_index_sha256':f.sha(public/'non-delete-evidence-index.json')})
            def source(a):f.save(packet/'source-freeze.json',{'source':a['source'],'recorded_only':True})
            calls=[]
            stack.enter_context(patch.object(f.r,'state',lambda:allocation));stack.enter_context(patch.object(f.r,'budget_completion',lambda a:None));stack.enter_context(patch.object(f.r,'preparation_gate',lambda a:None));stack.enter_context(patch.object(f,'source_freeze',source))
            stack.enter_context(patch.object(f,'preparation_audit_plan',lambda c:f.save(packet/'preparation-evidence-audit-plan.json',{'mode':'RECORDED_TEST_ONLY'})))
            stack.enter_context(patch.object(f.r,'run_owner',lambda cid,destination:calls.append(cid)))
            f.main()
            c=f.load(packet/'candidate.json');text=(packet/'confirmation-template.txt').read_text(encoding='utf-8')
            self.assertIn('--execution-binding-sha256',text)
            self.assertIn('未绑定不得授权或调度',text)
            self.assertIn('终态不自动解锁',text)
            self.assertEqual(c['Execution'],f.p.EXECUTION)
            self.assertEqual(text.count('删除目标 '),6);self.assertEqual(text.count('保留版本 '),11)
            self.assertNotIn('删除目标 dpp-lifecycle307-808a6960 dpp-lifecycle307-online/fixture/p1.csv',text)
            self.assertIn('全局至多六次',text);self.assertIn('2026-10-22 14:00–14:40',text)
            self.assertEqual(calls,[allocation['offline_validator']])
            self.assertEqual(f.load(packet/'status.json')['human_confirmation'],'NOT_RECEIVED')
            self.assertEqual(f.p.window.validate_packet(packet,control)['cases'],6)
            for g in c['Groups']:
                for stage in g['Helpers']:self.assertFalse(f.load(control/g['Group']/stage/'permission.json')['Enabled'])


if __name__=='__main__':unittest.main()
