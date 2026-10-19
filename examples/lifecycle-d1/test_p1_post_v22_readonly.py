"""Non-delete host contract regressions. Docker/IAM are mocked explicitly."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('p1_readonly',Path(__file__).with_name('reconcile-p1-post-v22.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class P1ReadOnlyHostTests(unittest.TestCase):
    def test_redis_explicit_image_user_and_owned_persistent_volume(self):
        v={'source':{'Network':'net'},'redis_volume':'owned-volume'}
        x={'Image':m.REDIS,'Config':{'User':'999:1000','Cmd':['redis-server','--appendonly','yes']},
           'State':{'Running':False,'Pid':0,'Status':'exited'},'HostConfig':{'NetworkMode':'net','ReadonlyRootfs':True,'Privileged':False,'CapDrop':['ALL'],'SecurityOpt':['no-new-privileges'],'RestartPolicy':{'Name':'no'},'PortBindings':{}},
           'NetworkSettings':{'Networks':{'net':{'NetworkID':'net'}},'Ports':{}},'Mounts':[{'Destination':'/data','Type':'volume','Name':'owned-volume','RW':True}]}
        m.redis_contract(x,v,False)
        created=copy.deepcopy(x);created['State']['Status']='created';created['NetworkSettings']['Networks']['net']['NetworkID']=''
        m.redis_contract(created,v,False)
        bad=copy.deepcopy(created);bad['State']['Status']='exited'
        with self.assertRaises(ValueError):m.redis_contract(bad,v,False)
        bad=copy.deepcopy(created);bad['HostConfig']['NetworkMode']='foreign'
        with self.assertRaises(ValueError):m.redis_contract(bad,v,False)
        created['State'].update(Running=True,Pid=1,Status='running')
        with self.assertRaises(ValueError):m.redis_contract(created,v,True)
        created['NetworkSettings']['Networks']['net']['NetworkID']='net'
        m.redis_contract(created,v,True)
        for label,change in [('default-root',lambda y:y['Config'].update(User='')),('wrong-volume',lambda y:y['Mounts'][0].update(Name='other')),('extra-network',lambda y:y['NetworkSettings']['Networks'].update(other={'NetworkID':'other'})),('port',lambda y:y['HostConfig'].update(PortBindings={'6379/tcp':[{}]}))]:
            with self.subTest(label=label):
                bad=copy.deepcopy(x);change(bad)
                with self.assertRaises(ValueError):m.redis_contract(bad,v,False)

    def test_redis_ping_readiness_checks_actual_running_owner_first(self):
        with tempfile.TemporaryDirectory() as d:
            v={'redis':'fixed-redis'}
            with patch.object(m,'PUBLIC',Path(d)),patch.object(m,'inspect',return_value={}),patch.object(m,'redis_contract'),patch.object(m.subprocess,'run',return_value=subprocess.CompletedProcess([],0,b'PONG\n',b'')) as ping:
                m.wait_redis(v)
                self.assertEqual(ping.call_args.args[0],['docker','exec','fixed-redis','redis-cli','--raw','PING'])
                self.assertEqual(m.load(Path(d)/'redis-ready.json')['ping'],'PONG')
            with patch.object(m,'inspect',return_value={}),patch.object(m,'redis_contract',side_effect=ValueError('exited queue')),patch.object(m.subprocess,'run') as ping:
                with self.assertRaises(ValueError):m.wait_redis(v)
                ping.assert_not_called()

    def test_manifest_paths_reject_traversal_absolute_and_alias_collisions(self):
        for inputs in ({'../root.json':'x'},{'/root.json':'x'},{'C:\\root.json':'x'},
                       {'a//b':'x'},{'a/./b':'x'},{'a\\b':'x','a/b':'x'}):
            with self.subTest(inputs=inputs),self.assertRaises(ValueError):
                list(m.input_entries({'inputs':inputs}))

    def test_actual_windows_manifest_survives_only_controlled_enabled_transition(self):
        with tempfile.TemporaryDirectory() as d:
            control=Path(d); originals={}
            for name in ('worker-private/recovery.json','root-private/p1-readonly-permission.json'):
                p=control/name;p.parent.mkdir(exist_ok=True)
                m.write(p,{'Enabled':False,'ExpiresAt':'fixed-end','Binding':'exact'})
                originals[name]=m.sha(p)
                (control/(p.stem+'-disabled-original.json')).write_bytes(p.read_bytes())
            host='dpp-lifecycle307-808a6960-postgres'
            g={'Core':'core','Native':'native','Network':'net'}
            manifest={'ExpiresAt':'2999-01-01T00:00:00+00:00','source_files':{},
                      'source':g,'redis':'redis','owners':{'worker':{'cid':'worker','private':str(control),'output':str(control),'binary':str(control/'binary'),'binary_sha256':'binary-sha'}},
                      'old_closed':{'owners':[],'old_control_sha256':{}},
                      'core_connection_binding':{'host':host},
                      'inputs':{name.replace('/','\\'):h for name,h in originals.items()}}
            def inspect(cid):
                return {'Name':'/'+(host if cid=='core' else cid),'Image':m.PG if cid=='core' else m.MI,
                        'State':{'Running':True},'NetworkSettings':{'Networks':{'net':{'NetworkID':'net','Aliases':None,'DNSNames':[host if cid=='core' else cid]}}}}
            # Actual profile bytes and hashes are checked; only Docker topology
            # and the independent immutable binary are modeled here.
            real_sha=m.sha
            def checked_sha(path):return 'binary-sha' if path==control/'binary' else real_sha(path)
            with patch.object(m,'CONTROL',control),patch.object(m,'inspect',side_effect=inspect),patch.object(m,'owner_contract'),patch.object(m,'redis_contract'),patch.object(m,'sha',side_effect=checked_sha),patch.object(m,'docker',return_value=json.dumps([{'Internal':True,'Containers':{'core':{},'native':{},'redis':{}}}]).encode()):
                for name in originals:
                    p=control/name;v=m.load(p);v['Enabled']=True;p.write_text(json.dumps(v))
                m.active_topology(manifest,'enable-p1-readonly')
                m.active_topology(manifest,'worker')
                p=control/'worker-private/recovery.json';v=m.load(p);v['ExpiresAt']='expanded-end';p.write_text(json.dumps(v))
                with self.assertRaises(ValueError):m.active_topology(manifest,'worker')
                v['ExpiresAt']='fixed-end';v['Enabled']=False;p.write_text(json.dumps(v))
                with self.assertRaises(ValueError):m.active_topology(manifest,'worker')
                for name in originals:
                    p=control/name;v=m.load(p);v['Enabled']=False;p.write_text(json.dumps(v))
                m.active_topology(manifest,'disable-all')

    def test_core_connection_binds_host_database_user_without_exposing_password(self):
        host='dpp-lifecycle307-808a6960-postgres'
        group={'Core':'fixed-Core','Copies':[{'WorkspaceID':'workspace','ProviderInstanceID':'provider','IncarnationID':'incarnation'}],'Actor':'actor'}
        base={'PostgresDSN':'postgres://fixture:test-password@'+host+':5432/lifecycle307?sslmode=disable','WorkspaceID':'workspace','ProviderInstanceID':'provider','IncarnationID':'incarnation','ActorID':'actor','Endpoint':'127.0.0.1:9000','InspectAccess':'inspect808a69606c36'}
        state={'Name':'/'+host,'NetworkSettings':{'Networks':{'net':{'Aliases':None,'DNSNames':[host]}}}}
        with patch.object(m,'inspect',return_value=state):
            proof=m.core_connection_binding(base,group)
            self.assertEqual(proof['Core'],'fixed-Core');self.assertNotIn('test-password',json.dumps(proof))
            for original,replacement in [(host,'another-Core'),('fixture:','root:'),('/lifecycle307?','/postgres?'),('sslmode=disable','sslmode=disable&service=other')]:
                bad=copy.deepcopy(base);bad['PostgresDSN']=bad['PostgresDSN'].replace(original,replacement)
                with self.assertRaises(ValueError):m.core_connection_binding(bad,group)

    def test_owner_failure_retains_original_stdout_and_stderr_separately(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);private=root/'private';private.mkdir();public=root/'public';public.mkdir()
            state={'State':{'Running':False,'Pid':0,'StartedAt':'start','FinishedAt':'end'},'HostConfig':{'PortBindings':{},'RestartPolicy':{'Name':'no'}},'NetworkSettings':{'Ports':{}}}
            command=subprocess.CompletedProcess(['docker','logs'],0,stdout=b'',stderr=b'original observation persistence error\n')
            with patch.object(m,'CONTROL',private),patch.object(m,'PUBLIC',public),patch.object(m,'active_topology'),patch.object(m,'docker',return_value=b''),patch.object(m,'text',return_value='1'),patch.object(m,'inspect',return_value=state),patch.object(m.subprocess,'run',return_value=command):
                with self.assertRaises(ValueError):m.execute_owner({'owners':{'worker':{'cid':'fixed-worker'}}},'worker')
            self.assertEqual((private/'worker-logs-stdout.private').read_bytes(),b'')
            self.assertEqual((private/'worker-logs-stderr.private').read_bytes(),command.stderr)
            receipt=m.load(public/'worker-owner.json');self.assertEqual(receipt['exit'],1);self.assertEqual(receipt['logs']['stderr']['bytes'],len(command.stderr))

    def test_complete_owner_contract_rejects_root_mount_port_and_command_expansion(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);private=root/'private';private.mkdir();output=root/'output';output.mkdir();binary=root/'worker';binary.write_bytes(b'worker')
            for name in ('audit.json','recovery.json','bindings.json'):(private/name).write_text('{}')
            x={'Image':m.PG,'State':{'Running':False,'Pid':0},'HostConfig':{'PortBindings':{},'RestartPolicy':{'Name':'no'},'NetworkMode':'container:source','ReadonlyRootfs':True,'Privileged':False,'CapDrop':['ALL'],'SecurityOpt':['no-new-privileges']},
               'NetworkSettings':{'Ports':{}},'Config':{'User':'10001:10001','Entrypoint':['/run/executor'],'Cmd':['p1-post-v22-readonly']},
               'Mounts':[{'Destination':dest,'Source':str(path),'Type':'bind','RW':rw} for dest,path,rw in [('/run/private',private,False),('/run/output',output,True),('/run/executor',binary,False)]]}
            m.owner_contract(x,'worker','source',private,output,binary)
            for label,mutate in {
                'root-user':lambda y:y['Config'].update(User='0'),
                'port':lambda y:y['HostConfig'].update(PortBindings={'9000/tcp':[{}]}),
                'mount':lambda y:y['Mounts'].append({'Destination':'/run/root','Source':'/root','Type':'bind','RW':False}),
                'command':lambda y:y['Config'].update(Cmd=['other']),
                'network':lambda y:y['HostConfig'].update(NetworkMode='host'),
                'restart':lambda y:y['HostConfig'].update(RestartPolicy={'Name':'always'}),
            }.items():
                with self.subTest(label=label):
                    bad=copy.deepcopy(x);mutate(bad)
                    with self.assertRaises(ValueError):m.owner_contract(bad,'worker','source',private,output,binary)
            (private/'root.json').write_text('{"RootSecret":"forbidden"}')
            with self.assertRaises(ValueError):m.owner_contract(x,'worker','source',private,output,binary)

    def test_seal_continues_all_stops_when_profiles_and_disable_readback_fail(self):
        manifest={'owners':{k:{'cid':k} for k in ('worker','enable-p1-readonly','disable-all')},'source':{'Core':'Core','Native':'Native'},'redis':'Redis'}
        stopped=[]
        def inspect(cid):
            if cid=='worker':raise RuntimeError('synthetic inspect failure')
            return {'State':{'Running':cid not in stopped,'Pid':0 if cid in stopped else 1},'HostConfig':{'PortBindings':{},'RestartPolicy':{'Name':'no'}},'NetworkSettings':{'Ports':{}}}
        def docker(*args):
            self.assertEqual(args[0],'stop');stopped.append(args[-1]);return b''
        with patch.object(m,'revoke_profiles',return_value=[{'profile':'broken','disabled':False}]),patch.object(m,'inspect',side_effect=inspect),patch.object(m,'docker',side_effect=docker),patch.object(m,'execute_owner',side_effect=RuntimeError('synthetic disable failure')) as disable:
            result=m.seal(manifest)
        self.assertFalse(result['all_closed']);self.assertFalse(result['fresh_disabled_readback'])
        self.assertEqual(disable.call_args.args[1],'disable-all')
        self.assertEqual(set(stopped),{'worker','enable-p1-readonly','disable-all','Redis','Native','Core'})

    def test_seal_attempts_every_bound_stop_even_when_every_inspect_is_unreadable(self):
        manifest={'owners':{k:{'cid':k} for k in ('worker','enable-p1-readonly','disable-all')},'source':{'Core':'Core','Native':'Native'},'redis':'Redis'}
        stopped=[]
        with patch.object(m,'revoke_profiles',return_value=[{'profile':'revoked','disabled':True}]),patch.object(m,'inspect',side_effect=RuntimeError('unreadable')),patch.object(m,'docker',side_effect=lambda *a:stopped.append(a[-1])),patch.object(m,'execute_owner',side_effect=RuntimeError('disable unknown')):
            result=m.seal(manifest)
        self.assertEqual(set(stopped),{'worker','enable-p1-readonly','disable-all','Redis','Native','Core'})
        self.assertFalse(result['all_closed']);self.assertFalse(result['fresh_disabled_readback'])
        self.assertTrue(all(not v['closed'] and v['initial_inspect_failed'] for v in result['closed']))

    def test_prestart_failure_is_terminal_and_starts_no_source_or_identity(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);control=root/'private';public=root/'public';control.mkdir();public.mkdir()
            m.write(control/'manifest.json',{'audit_id':'fixed-test-audit'})
            review=root/'review.json';m.write(review,{'status':'PASS_BOUNDED_INDEPENDENT_P1_READONLY_REVIEW','remaining_blockers':[],'manifest_sha256':m.sha(control/'manifest.json')})
            with patch.object(m,'CONTROL',control),patch.object(m,'PUBLIC',public),patch.object(m,'fresh',side_effect=ValueError('synthetic drift')),patch.object(m,'revoke_profiles'),patch.object(m,'docker') as docker:
                m.run(review,m.sha(review))
            docker.assert_not_called()
            terminal=m.load(control/'terminal.json');self.assertEqual(terminal['status'],'BLOCKED_PRESTART_NO_SOURCE_IO');self.assertFalse(terminal['identity_enabled'])
            with patch.object(m,'CONTROL',control),self.assertRaises(ValueError):m.run(review,m.sha(review))

if __name__=='__main__':unittest.main(verbosity=2)
