import copy
import json
from pathlib import Path
import unittest
import io
import tarfile

import remaining_sequence as sequence

ROOT = Path(__file__).resolve().parents[2]


class SixSequence(unittest.TestCase):
    def setUp(self):
        self.candidate = json.loads((ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate12/candidate.json').read_bytes())
        self.receipts, self.owners = {}, {}
        for g in self.candidate['Groups']:
            g['Helpers'].pop('P1', None); g['Helpers'].pop('recover-P1', None)
            self.receipts[g['Copies'][0]['WorkspaceID']] = set()
            for stage, cid in g['Helpers'].items():
                self.owners[cid] = ({'Id': cid, 'State': {'Running': False, 'Pid': 0}}, {'Enabled': False})
        p1 = next(c for c in self.candidate['Groups'][0]['Copies'] if c['Key'] == 'fixture/p1.csv')
        self.receipts[p1['WorkspaceID']].add(p1['ID'])

    def evaluate(self, group, stage):
        return sequence.evaluate(self.candidate, group, stage, self.receipts, self.owners)

    def test_six_complete_send_disable_recover_commit_composition(self):
        for n, (group, stage, cp) in enumerate(sequence.targets(self.candidate)):
            for selected in ('wire', stage):
                self.assertEqual(self.evaluate(group, selected)['slot'], n)
            # The model has no provider; SDK/wire has a separate Go composition.
            self.assertEqual(self.evaluate(group, 'recover-'+stage)['completed'], n)
            self.receipts[cp['WorkspaceID']].add(cp['ID'])
            self.assertEqual(self.evaluate(group, 'recover-'+stage)['completed'], n+1)
            with self.assertRaises(ValueError): self.evaluate(group, stage)
        self.assertEqual(sum(len(v) for v in self.receipts.values()), 7)
        with self.assertRaises(ValueError): self.evaluate('mixed', 'wire')

    def test_no_skip_cross_group_or_receipt_hole(self):
        for group, stage, _ in sequence.targets(self.candidate)[1:]:
            with self.subTest(stage=stage), self.assertRaises(ValueError): self.evaluate(group, stage)
        group, stage, cp = sequence.targets(self.candidate)[3]
        self.receipts[cp['WorkspaceID']].add(cp['ID'])
        with self.assertRaises(ValueError): self.evaluate('business-races', 'P2')

    def test_sender_or_permission_not_closed_blocks_recovery_and_next_group(self):
        targets = sequence.targets(self.candidate)
        for _, _, cp in targets[:3]: self.receipts[cp['WorkspaceID']].add(cp['ID'])
        g = self.candidate['Groups'][0]; cid = g['Helpers']['T2']
        for field, value in (('Running', True), ('Pid', 42)):
            with self.subTest(field=field):
                self.owners[cid][0]['State'][field] = value
                with self.assertRaises(ValueError): self.evaluate('late-hold', 'H2')
                self.owners[cid][0]['State'][field] = False if field == 'Running' else 0
        self.owners[cid][1]['Enabled'] = True
        with self.assertRaises(ValueError): self.evaluate('late-hold', 'H2')
        with self.assertRaises(ValueError): self.evaluate('business-races', 'recover-T2')

    def test_p1_foreign_workspace_substitution_and_unknown_helper_reject(self):
        with self.assertRaises(ValueError): self.evaluate('business-races', 'P1')
        with self.assertRaises(ValueError): self.evaluate('business-races', 'recover-P1')
        g = self.candidate['Groups'][0]; cid = g['Helpers']['P2']
        self.owners[cid][0]['Id'] = '0'*64
        with self.assertRaises(ValueError): self.evaluate('business-races', 'recover-P2')
        self.owners[cid][0]['Id'] = cid
        self.receipts[g['Copies'][0]['WorkspaceID']] = set()
        with self.assertRaises(ValueError): self.evaluate('business-races', 'P2')

    def archive(self,*,uid=10001,filename=None,payload=None):
        stream=io.BytesIO()
        with tarfile.open(fileobj=stream,mode='w') as archive:
            root=tarfile.TarInfo('budget');root.type=tarfile.DIRTYPE;root.uid=root.gid=uid;root.mode=0o755;archive.addfile(root)
            if filename:
                raw=json.dumps(payload).encode();item=tarfile.TarInfo('budget/'+filename);item.uid=item.gid=uid;item.mode=0o600;item.size=len(raw);archive.addfile(item,io.BytesIO(raw))
        return stream.getvalue()

    def test_actual_budget_owner_empty_and_burned_slot_contract(self):
        cp=sequence.targets(self.candidate)[0][2]
        payload={'execution':self.candidate['Execution'],'copy':cp,'attempt':{'CopyID':cp['ID'],'WorkspaceID':cp['WorkspaceID']}}
        result=sequence.budget_archive(self.archive(),self.candidate,'business-races',initial=True)
        self.assertEqual(result['files'],[])
        raw=self.archive(filename=cp['ID']+'.json',payload=payload)
        self.assertEqual(len(sequence.budget_archive(raw,self.candidate,'business-races',initial=False)['files']),1)
        with self.assertRaises(ValueError):sequence.budget_archive(raw,self.candidate,'business-races',initial=True)

    def test_budget_wrong_type_root_owner_foreign_execution_and_p1_reject(self):
        with self.assertRaises(ValueError):sequence.budget_archive('wrong type',self.candidate,'business-races',initial=True)
        with self.assertRaises(ValueError):sequence.budget_archive(self.archive(uid=0),self.candidate,'business-races',initial=True)
        cp=sequence.targets(self.candidate)[0][2]
        payload={'execution':'old','copy':cp,'attempt':{'CopyID':cp['ID'],'WorkspaceID':cp['WorkspaceID']}}
        with self.assertRaises(ValueError):sequence.budget_archive(self.archive(filename=cp['ID']+'.json',payload=payload),self.candidate,'business-races',initial=False)
        with self.assertRaises(ValueError):sequence.budget_archive(self.archive(filename='5a4c9789-3e67-4e54-ab58-128d111cbd65.json',payload=payload),self.candidate,'business-races',initial=False)


if __name__ == '__main__': unittest.main()
