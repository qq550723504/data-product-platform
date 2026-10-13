"""Driver boundary regressions. No Docker or native provider is invoked."""
import datetime as dt
import copy
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

DRIVER=Path(__file__).with_name('run-confirmed-race.py')
spec=importlib.util.spec_from_file_location('confirmed_race',DRIVER)
driver=importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)


class DriverBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name)/'checkout'
        self.private=Path(self.temp.name)/driver.PROJECT
        self.control=self.private/'race-control'
        self.out=self.root/'examples/lifecycle-d1/results'/driver.RUN
        self.out.mkdir(parents=True)
        self.private.mkdir()
        self.control.mkdir()
        for key,value in dict(root=self.root,private=self.private,control=self.control,
                              out=self.out,c={'WorkspaceID':'fixture-workspace'},
                              r={'containers':{'gateway':'owned-gateway'}},
                              gateway_changed=False).items():
            p=patch.object(driver,key,value,create=True)
            p.start()
            self.addCleanup(p.stop)

    def args(self,phase='a'):
        return [phase,'--private-directory',str(self.private),'--confirmed-plan',driver.HASH]

    def permission(self,enabled=True,expired=False):
        start=dt.datetime.now(dt.timezone.utc)-dt.timedelta(minutes=10)
        end=start+dt.timedelta(minutes=5 if expired else 40)
        for folder in ['executor','admin']:
            target=self.control/folder
            target.mkdir(exist_ok=True)
            (target/'delete-permission.json').write_text(json.dumps({
                'Enabled':enabled,'RunID':driver.RUN,'WorkspaceID':'fixture-workspace',
                'PlanHash':driver.HASH,'ConfirmedAt':start.isoformat(),'ExpiresAt':end.isoformat()
            }))

    def test_sealed_scope_all_phases_reject_before_private_reads_or_io(self):
        (self.out/'confirmed-race-sealed.json').write_text('{}')
        with patch.object(driver,'docker') as docker, patch.object(driver,'gateway') as gateway:
            for phase in driver.PHASES:
                with self.subTest(phase=phase), self.assertRaisesRegex(RuntimeError,'approval consumed'):
                    driver.main(self.args(phase))
            docker.assert_not_called()
            gateway.assert_not_called()
        self.assertFalse((self.private/'Caddyfile').exists())

    def test_bad_confirmation_rejects_before_private_reads_or_io(self):
        args=self.args();args[-1]='other'
        with patch.object(driver,'docker') as docker, self.assertRaises(RuntimeError):
            driver.main(args)
        docker.assert_not_called()

    def test_expired_or_disabled_authority_never_changes_gateway(self):
        for enabled,expired in [(False,False),(True,True)]:
            self.permission(enabled,expired)
            with patch.object(driver,'load_driver'), patch.object(driver,'docker') as docker, \
                 patch.object(driver,'gateway') as gateway:
                for phase in ['a','b','a-resume','a-reader-first','provision','provision-resume','inspect','seal']:
                    with self.subTest(enabled=enabled,expired=expired,phase=phase), self.assertRaises(RuntimeError):
                        driver.main(self.args(phase))
                docker.assert_not_called()
                gateway.assert_not_called()

    def test_failed_helper_closes_all_object_and_admin_routes(self):
        self.permission()
        (self.control/'gateway-fault-active').write_text('ACTIVE EXACT DELETE ROUTE')
        with patch.object(driver,'load_driver'), patch.object(driver,'runtime_preflight'), \
             patch.object(driver,'docker',return_value=b'') as docker, \
             patch.object(driver,'helper',side_effect=RuntimeError('helper failed')):
            with self.assertRaisesRegex(RuntimeError,'helper failed'):
                driver.main(self.args())
        self.assertEqual((self.private/'Caddyfile').read_text(),driver.DENY_ALL_GATEWAY)
        self.assertNotIn('@admin',driver.DENY_ALL_GATEWAY)
        self.assertNotIn('@payload',driver.DENY_ALL_GATEWAY)
        self.assertEqual(sum(args.args[0]=='restart' for args in docker.call_args_list),2)

    def test_runtime_topology_and_schema_rejections_cannot_open_gateway(self):
        containers={role:driver.PROJECT+'-'+role for role in ['postgres','minio','gateway']}
        items=[]
        for role,name in containers.items():
            item={'Name':'/'+name,'Id':role+'-id','Image':role+'-image',
                  'Config':{'Labels':{'lifecycle307.run':driver.RUN}},'State':{'Running':True},
                  'NetworkSettings':{'Ports':{}},'HostConfig':{},'Mounts':[]}
            if role!='gateway':
                volume='postgres' if role=='postgres' else 'objects'
                destination='/var/lib/postgresql/data' if role=='postgres' else '/data'
                item['Mounts']=[{'Destination':destination,'Type':'volume',
                                 'Name':driver.PROJECT+'-'+volume,'RW':True}]
            items.append(item)
        network={'Id':'network-id','Internal':True,'Containers':{
            str(i):{'Name':name} for i,name in enumerate([*containers.values(),driver.PROJECT+'-race-redis'])}}
        driver.r={'containers':containers,'network':'owned-network'}
        driver.m={'environment':{'containers':[{'name':v['Name'].lstrip('/'),'id':v['Id'],
                                              'image':v['Image']} for v in items],
                                'network':{'id':'network-id'},'minio_binary_sha256':'native-hash'}}
        self.permission()
        for bad in ['id','image','port','volume','tmpfs','network','schema']:
            candidate=copy.deepcopy(items);net=copy.deepcopy(network)
            if bad=='id':candidate[0]['Id']='foreign-id'
            if bad=='image':candidate[0]['Image']='foreign-image'
            if bad=='port':candidate[0]['NetworkSettings']['Ports']={'5432/tcp':[{'HostPort':'5432'}]}
            if bad=='volume':candidate[0]['Mounts'][0]['Name']='foreign-volume'
            if bad=='tmpfs':candidate[0]['HostConfig']['Tmpfs']={'/var/lib/postgresql/data':''}
            if bad=='network':net['Internal']=False
            def docker(*args):
                if args[0]=='inspect':return json.dumps(candidate).encode()
                if args[0]=='network':return json.dumps([net]).encode()
                return b'native-hash /usr/local/bin/minio'
            with self.subTest(bad=bad), patch.object(driver,'load_driver'), \
                 patch.object(driver,'docker',side_effect=docker), \
                 patch.object(driver,'sql',return_value='65' if bad=='schema' else '66'), \
                 patch.object(driver,'gateway') as gateway, patch.object(driver,'helper') as helper:
                with self.assertRaises(RuntimeError):driver.main(self.args())
                gateway.assert_not_called()
                helper.assert_not_called()

    def test_bad_backup_and_prior_attempt_cannot_prepare_authority(self):
        self.control.rmdir()
        backup=self.private/'backup-after-confirmation'
        backup.mkdir()
        for status,attempts in [('FAIL','0'),('PASS','1')]:
            (backup/'result.json').write_text(json.dumps({'status':status,'objectVersions':4,
                                                         'fullFactsEqualIndependentRestore':True}))
            with self.subTest(status=status,attempts=attempts), patch.object(driver,'load_driver'), \
                 patch.object(driver,'sql',return_value=attempts), patch.object(driver,'docker') as docker:
                with self.assertRaises(RuntimeError):driver.main(self.args('prepare'))
                docker.assert_not_called()
            self.assertFalse(self.control.exists())
            self.assertFalse((self.out/'confirmed-race-human-approval.json').exists())

    def test_optimized_interpreter_enforces_confirmation_and_consumed_scope(self):
        # Executing the real entry point against its committed sealed receipt
        # must reject before looking up any private config or spawning Docker.
        for plan,expected in [('other','Exact human-confirmed plan required'),
                              (driver.HASH,'approval consumed')]:
            for phase in driver.PHASES:
                with self.subTest(plan=plan,phase=phase):
                    r=subprocess.run([sys.executable,'-O',str(DRIVER),phase,
                                      '--private-directory',str(self.private),
                                      '--confirmed-plan',plan],capture_output=True,text=True)
                    self.assertNotEqual(r.returncode,0)
                    self.assertIn(expected,r.stderr)
                    self.assertNotIn('FileNotFoundError',r.stderr)
                    self.assertNotIn('CalledProcessError',r.stderr)


if __name__=='__main__':unittest.main()
