"""Complete-entry offline composition. No Docker/Core/provider operation is real."""
from contextlib import ExitStack
import copy
import datetime as dt
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('seventeen',Path(__file__).with_name('verify-post-v22-seventeen.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class SeventeenTests(unittest.TestCase):
    def composition(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control,public=Path(tmp)/'private',Path(tmp)/'public';control.mkdir();public.mkdir()
            stack.enter_context(patch.object(m,'CONTROL',control));stack.enter_context(patch.object(m,'PUBLIC',public))
            groups=m.load(m.p1.PACKET/'candidate.json')['Groups']
            states={};owners={};calls=[]
            baseline=m.load(m.ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03/Core-after.json')
            core_calls={};source_baselines={g['Core']:(baseline if g['Group']=='business-races' else m.load(m.p1.PACKET/g['Group']/'core-readonly.json')) for g in groups}
            for g in groups:
                name=g['Group'];private=control/name;output=public/name/'readonly';private.mkdir();output.mkdir(parents=True)
                profile=dict(Enabled=False,MaxProviderDeletes=0,NotBefore='0001-01-01T00:00:00Z',ExpiresAt='0001-01-01T00:00:00Z',
                    CandidateSHA256='c'*64,FrozenInventorySHA256='f'*64,PresentInventorySHA256='e'*64)
                m.save(private/'post-v22-readonly.json',profile);m.save(control/(name+'-disabled-original.json'),profile)
                owners[name]=dict(cid='helper-'+name,Core=g['Core'],Native=g['Native'],Network=g['Network'],private=str(private),output=str(output),binary_sha256='b'*64)
                for cid in (g['Core'],g['Native'],'helper-'+name):states[cid]={'State':{'Running':False,'Pid':0},'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}
            manifest=dict(audit_id='synthetic-audit',source_head='synthetic-source',owners=owners,preservation={})
            m.save(control/'manifest.json',manifest);m.save(public/'preparation.json',manifest)
            digest=m.sha(control/'manifest.json')
            review=public/'independent.json';m.save(review,{'status':'PASS_BOUNDED_INDEPENDENT_REVIEW','remaining_blockers':[],'manifest_sha256':digest,'source_head':manifest['source_head']})

            def inspect(cid):
                calls.append(('inspect',cid))
                if fault=='inspect-unknown' and cid=='helper-business-races':raise RuntimeError('modeled inspect unavailable')
                return states[cid]
            def docker(*args):
                calls.append(args)
                if args[0]=='network':
                    g=next(g for g in groups if g['Network']==args[2]);return json.dumps([{'Internal':True,'Containers':{g['Core']:{},g['Native']:{}}}]).encode()
                cid=args[-1]
                if args[0]=='start':
                    states[cid]['State'].update(Running=True,Pid=1,StartedAt=m.now().isoformat())
                    if fault=='source-start' and cid==groups[0]['Core']:raise RuntimeError('modeled start unreadable after effect')
                    if cid.startswith('helper-'):
                        owner=next(o for o in owners.values() if o['cid']==cid)
                        if not m.load(Path(owner['private'])/'post-v22-readonly.json')['Enabled']:raise AssertionError('profile not enabled')
                        directory=Path(owner['output'])
                        m.save(directory/'readonly-start-request.json',{'id':'request','method':'GET'})
                        m.save(directory/'readonly-observation-request.json',{'id':'request','status':200,'transport_error':False})
                if args[0]=='stop':states[cid]['State'].update(Running=False,Pid=0)
                return b''
            def owner_contract(owner,allow_running=False):
                x=inspect(owner['cid'])
                if not allow_running and x['State']['Running']:raise AssertionError('owner running')
                return x
            def run(args,**kwargs):
                calls.append(('subprocess',*args))
                if args[0]=='docker' and args[1]=='wait':
                    if fault=='wait-timeout':raise subprocess.TimeoutExpired(args,1,output=b'wait-partial',stderr=b'wait-error')
                    cid=args[2];states[cid]['State'].update(Running=False,Pid=0,FinishedAt=m.now().isoformat(),ExitCode=0)
                    return SimpleNamespace(returncode=0,stdout=b'0\n',stderr=b'')
                if args[0]=='docker' and args[1]=='logs':
                    if fault=='logs-timeout':raise subprocess.TimeoutExpired(args,1,output=b'log-partial',stderr=b'log-error')
                    return SimpleNamespace(returncode=0,stdout=b'bounded-native-mock',stderr=b'')
                if '-post-v22' not in args:raise AssertionError('legacy checker used')
                if fault=='checker-timeout':raise subprocess.TimeoutExpired(args,1,output=b'checker-partial',stderr=b'checker-error')
                if fault=='checker-reject':return SimpleNamespace(returncode=1,stdout=b'',stderr=b'original-counterexample')
                group=args[args.index('-group')+1]
                return SimpleNamespace(returncode=0,stdout=json.dumps({'status':'PASS_COMPLETE_POST_V22_NATIVE_AGGREGATION','group':group,'copies':m.COUNTS[m.GROUPS.index(group)]}).encode(),stderr=b'')
            def fingerprints(cid):
                core_calls[cid]=core_calls.get(cid,0)+1
                if fault=='Core-drift' and core_calls[cid]>1:return {'drift':'modeled'}
                return source_baselines[cid]
            original_replace=m.replace_profile
            def replace_profile(path,value):
                if fault=='profile-revoke' and value['Enabled'] is False:raise OSError('modeled revoke failure')
                original_replace(path,value)
            def fresh(manifest):
                if fault=='precondition':raise ValueError('modeled retained budget mismatch')
            actual_now=m.now
            def clock():
                if fault=='shared-expired' and core_calls.get(groups[0]['Core'],0)>1:return actual_now()+dt.timedelta(minutes=56)
                return actual_now()
            for obj,name,value in [(m,'fresh',fresh),(m,'retained',lambda:{}),(m,'inspect',inspect),(m.p1,'inspect',inspect),
                (m,'docker',docker),(m.p1,'docker',docker),(m,'owner_contract',owner_contract),(m,'replace_profile',replace_profile),
                (m.closure,'source_contract',lambda _:None),(m.native,'wait_pg',lambda *_:None),(m.native,'fingerprints',fingerprints),
                (m.subprocess,'run',run),(m.time,'sleep',lambda _:None),(m,'now',clock),(m,'print',lambda *args,**kwargs:None)]:stack.enter_context(patch.object(obj,name,value))
            if fault:
                with self.assertRaises(ValueError):m.run(review,m.sha(review),digest)
            else:m.run(review,m.sha(review),digest)
            terminal=m.load(public/'terminal.json')
            self.assertTrue(all(not s['State']['Running'] for s in states.values()) or fault=='inspect-unknown')
            if fault!='profile-revoke':self.assertTrue(all(m.load(Path(o['private'])/'post-v22-readonly.json')['Enabled'] is False for o in owners.values()))
            self.assertEqual(terminal['native_delete'],0)
            self.assertFalse(any('DELETE' in call or 'enable' in call for call in calls))
            if fault=='checker-timeout':
                self.assertEqual((public/'business-races/wait-stdout.txt').read_bytes(),b'0\n')
                self.assertEqual((public/'business-races/checker-stdout.txt').read_bytes(),b'checker-partial')
            if fault=='logs-timeout':self.assertEqual((public/'business-races/stdout.txt').read_bytes(),b'log-partial')
            if fault=='wait-timeout':self.assertEqual((public/'business-races/wait-stdout.txt').read_bytes(),b'wait-partial')
            if fault and fault!='precondition':
                self.assertNotIn('business-races',terminal['not_run_groups'],'attempted group was mislabeled NOT_RUN')
            if fault=='checker-reject':
                self.assertEqual(terminal['failed_groups'],['business-races'])
                self.assertEqual(terminal['not_run_groups'],['late-hold','mixed'])
                self.assertEqual(terminal['request_journals']['business-races']['start_facts'],1)
            if not fault:
                windows={(m.load(public/g/'expectation.json')['not_before'],m.load(public/g/'expectation.json')['expires_at']) for g in m.GROUPS}
                self.assertEqual(len(windows),1,'group collection shifted the shared audit deadline')
            # Terminal evidence makes this exact run one-shot even with a valid review.
            before=len(calls)
            with self.assertRaises(ValueError):m.run(review,m.sha(review),digest)
            self.assertEqual(len(calls),before)
            return terminal,calls

    def test_full_entry_success_all_three_groups(self):
        result,calls=self.composition()
        self.assertEqual(result['status'],'PASS_COMPLETE_POST_V22_SEVENTEEN_READONLY')
        self.assertEqual(result['versions'],17);self.assertEqual(result['technical_requests'],3)
        self.assertEqual([g['group'] for g in result['groups']],list(m.GROUPS))
        self.assertEqual(result['D1'],'NOT_ACCEPTED')

    def test_whole_entry_failures_close_and_stop_later_groups(self):
        for fault in ('source-start','Core-drift','wait-timeout','checker-timeout','checker-reject','logs-timeout','precondition','profile-revoke','inspect-unknown','shared-expired'):
            with self.subTest(fault=fault):
                result,calls=self.composition(fault)
                self.assertNotEqual(result['status'],'PASS_COMPLETE_POST_V22_SEVENTEEN_READONLY')
                self.assertFalse(any(c==('start','helper-late-hold') for c in calls))
                for group in m.GROUPS:self.assertTrue(any(c==('inspect','helper-'+group) for c in calls))

    def test_budget_complete_ownership_and_mount_membership(self):
        volume={'Name':'old-budget','Labels':{'lifecycle307.remaining':'old-execution'}}
        group={'Helpers':{'send':'s','wire':'w'}};binding={'budget_volume':'old-budget'}
        self.assertEqual(m.budget_contract(volume,['w','s'],group,binding,'old-execution')['mounted_cids'],['s','w'])
        for fault in ('owner','name','extra','missing','duplicate'):
            v=copy.deepcopy(volume);users=['s','w']
            if fault=='owner':v['Labels']['lifecycle307.remaining']='new-execution'
            if fault=='name':v['Name']='foreign'
            if fault=='extra':users+=['unexpected']
            if fault=='missing':users=['s']
            if fault=='duplicate':users+=['s']
            with self.subTest(fault=fault),self.assertRaises(ValueError):m.budget_contract(v,users,group,binding,'old-execution')

    def test_unreadable_inspect_still_stops_known_cid_and_continues(self):
        calls=[]
        def inspect(cid):raise RuntimeError('unreadable')
        with patch.object(m.p1,'inspect',inspect),patch.object(m.p1,'docker',lambda *args:calls.append(args)):
            result=m.close_all(['one','two'])
        self.assertEqual(calls,[('stop','--time','15','two'),('stop','--time','15','one')])
        self.assertTrue(all(not r['closed'] and r['initial_inspect_failed'] for r in result))


if __name__=='__main__':unittest.main()
