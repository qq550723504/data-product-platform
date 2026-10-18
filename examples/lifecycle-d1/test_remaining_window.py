"""R1 cross-field tests exercise generator, actual complete CLI and offline gate.

Every semantic mutation is rehashed; no Docker, secrets or provider I/O.
"""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

def module(name, file):
    spec=importlib.util.spec_from_file_location(name,Path(__file__).with_name(file))
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

f=module('r1_freezer','freeze-remaining-executable.py')
v=module('r1_complete','verify-remaining-native.py')
w=f.window
a=module('r1_offline','check-remaining-candidate12.py')

def save(path,value):
    path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(value),encoding='utf-8')
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()

class WindowContractTests(unittest.TestCase):
    def fixture(self,root):
        packet=root/'packet';owner=root/'owner';owner.mkdir()
        (owner/'reader').write_bytes(b'no runtime');(owner/'checker').write_bytes(b'no provider')
        c={key:value for key,value in w.compiled().items() if key!='RecoveryEnd'}
        c['ReaderBinarySHA256']=sha(owner/'reader');c['Groups']=[]
        receipt={'cases':[],'recoveries':[]}
        recovery={'business-races':['P1','P2','T1','T2'],'late-hold':['H2'],'mixed':['M1A','M1B']}
        for group,modes in w.CASE_MODES.items():
            g={'Group':group,'CaseHelpers':{},'CaseConfigSHA256':{},'Helpers':{},'PolicyHeads':{str(i):str(i) for i in range(6)} if group=='business-races' else {}}
            for stage in modes:
                p=w.case_profile({'Base':{},'Group':group},g['PolicyHeads'],'not-a-credential','test')
                path=owner/group/('case-'+stage)/'remaining.json';save(path,p)
                g['CaseHelpers'][stage]='test-'+stage;g['CaseConfigSHA256'][stage]=sha(path)
                receipt['cases'].append(w.project_case(p,group,stage,sha(path)))
            for stage in recovery[group]:
                stage='recover-'+stage;g['Helpers'][stage]='test-'+stage
                p={'ValidUntil':w.compiled()['RecoveryEnd']};path=owner/group/stage/'recovery.json';save(path,p)
                receipt['recoveries'].append({'group':group,'stage':stage,'config_sha256':sha(path),**p})
            c['Groups'].append(g)
        save(packet/'bindings.json',[]);save(packet/'profile-windows.json',receipt)
        c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');save(packet/'candidate.json',c)
        return packet,owner,c,receipt

    def test_generator_overwrites_old_deadline_and_does_not_mutate_template(self):
        template={'Base':{'StartedAt':'old','ExpiresAt':'old','RootAccess':'private','RootSecret':'private'},'CandidateUseUntil':'2026-10-17T02:40:00Z','Batch':'old','Group':'business-races'}
        before=copy.deepcopy(template)
        actual=f.window.case_profile(template,{str(i):str(i) for i in range(6)},'new','epoch')
        self.assertEqual(actual['CandidateUseUntil'],f.END)
        self.assertEqual(actual['Base']['ExpiresAt'],actual['CandidateUseUntil'])
        self.assertEqual(actual['Base']['StartedAt'],f.START)
        self.assertEqual(actual['Batch'],f.EXECUTION)
        self.assertNotIn('RootSecret',actual['Base']);self.assertEqual(template,before)

    def test_generated_profiles_pass_public_and_actual_file_checks(self):
        with tempfile.TemporaryDirectory() as temp:
            packet,owner,_,_=self.fixture(Path(temp))
            self.assertEqual(w.validate_packet(packet)['cases'],7)
            self.assertEqual(w.validate_packet(packet,owner)['recoveries'],7)

    def mutate(self,root,kind):
        packet,owner,c,r=self.fixture(root);row=r['cases'][0]
        if kind=='use_until':row['CandidateUseUntil']='2026-10-17T02:40:00Z'
        elif kind=='use_null':row['CandidateUseUntil']=None
        elif kind=='use_before':row['CandidateUseUntil']='2026-10-18T14:39:59Z'
        elif kind=='use_after':row['CandidateUseUntil']='2026-10-18T14:40:01Z'
        elif kind=='base_end':row['Base']['ExpiresAt']='2026-10-18T14:41:00Z'
        elif kind=='base_start':row['Base']['StartedAt']='2026-10-18T13:59:00Z'
        elif kind=='execution':row['Batch']='67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5'
        elif kind=='candidate_end':c['NotAfter']='2026-10-18T15:40:00Z'
        elif kind=='recovery':r['recoveries'][0]['ValidUntil']='2026-10-18T16:41:00Z'
        elif kind=='membership':r['cases'][-1]=copy.deepcopy(row)
        elif kind=='heads':c['Groups'][0]['PolicyHeads'].pop('0')
        elif kind in ('actual_only','projection_lie','actual_heads','actual_group'):
            path=owner/row['group']/('case-'+row['stage'])/'remaining.json';p=json.loads(path.read_bytes())
            if kind=='actual_heads':p['ExpectedPolicyHeads']['0']='foreign'
            elif kind=='actual_group':p['Group']='mixed'
            else:p['CandidateUseUntil']='2026-10-17T02:40:00Z'
            save(path,p)
            if kind!='actual_only':
                row['config_sha256']=sha(path);c['Groups'][0]['CaseConfigSHA256'][row['stage']]=sha(path)
        else:raise AssertionError(kind)
        # Attacker/bug may recompute every transport hash. Semantic mismatch
        # must still fail the real entry before sealed_gate or collect_group.
        if kind in ('use_until','use_null','use_before','use_after','base_end','base_start','execution'):
            path=owner/row['group']/('case-'+row['stage'])/'remaining.json';p=json.loads(path.read_bytes())
            p.update({key:row[key] for key in ('Batch','CandidateUseUntil','Base')});save(path,p)
            row['config_sha256']=sha(path);c['Groups'][0]['CaseConfigSHA256'][row['stage']]=sha(path)
        save(packet/'profile-windows.json',r);c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');save(packet/'candidate.json',c)
        return packet,owner

    def test_semantic_mutations_fail_even_after_rehash_offline(self):
        for kind in ('use_until','use_null','use_before','use_after','base_end','base_start','execution','candidate_end','recovery','membership','heads'):
            with self.subTest(kind=kind),tempfile.TemporaryDirectory() as temp:
                packet,_=self.mutate(Path(temp),kind)
                with self.assertRaises((ValueError,KeyError)):w.validate_packet(packet)
                with patch.object(a,'PACKET',packet):
                    with self.assertRaises((ValueError,KeyError)):a.check()

    def test_complete_cli_rejects_cross_fields_before_permission_or_source_io(self):
        for kind in ('use_until','use_null','use_before','use_after','base_end','base_start','execution','candidate_end','recovery','membership','heads','actual_only','projection_lie','actual_heads','actual_group'):
            with self.subTest(kind=kind),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);packet,owner=self.mutate(root,kind)
                argv=['verify-remaining-native.py','--packet',str(packet),'--candidate-sha256',sha(packet/'candidate.json'),'--bindings-sha256',sha(packet/'bindings.json'),
                      '--private',str(root/'collection'),'--output',str(root/'output'),'--reader',str(owner/'reader'),'--checker',str(owner/'checker'),'--checker-sha256',sha(owner/'checker')]
                with patch.object(sys,'argv',argv),patch.object(v,'sealed_gate') as sealed,patch.object(v,'collect_group') as collect,patch.object(v.runner,'docker') as docker:
                    with self.assertRaises((ValueError,KeyError)):v.main()
                sealed.assert_not_called();collect.assert_not_called();docker.assert_not_called()
                self.assertFalse((root/'collection').exists());self.assertFalse((root/'output').exists())

if __name__=='__main__':unittest.main(verbosity=2)
