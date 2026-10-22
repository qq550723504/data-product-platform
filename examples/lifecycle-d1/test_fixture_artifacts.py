"""Real Git archives, isolated fake compiler. No Docker/provider/database."""
import copy
import io
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import zipfile

import fixture_artifacts as f

class ArtifactTests(unittest.TestCase):
    def fixture(self,root):
        subprocess.run(['git','init','-q',str(root)],check=True)
        def commit():
            f.git(root,'add','.')
            f.git(root,'-c','user.name=Artifact Test','-c','user.email=artifact@example.invalid','commit','-qm','fixture')
            return f.git(root,'rev-parse','HEAD').decode().strip()
        def write(name,raw):
            p=root/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(raw)
        write('apps/platform/repository.go',b'fixed schema 69\n')
        write('apps/platform/scope.json',b'{"window":"new"}\n')
        write('migrations/69.sql',b'fixed historical DDL\n')
        base=commit();write('apps/platform/fixture.go',b'new execution, exact schema guard\n');source=commit()
        raw=f.git(root,'archive','--format=zip',source,'apps/platform','migrations')
        spec=dict(version=1,purpose='RETAINED_D1_FIXTURE_ONLY',execution='new',profile='test/v1',not_before='start',not_after='end',
                  schema=dict(max_version=69,migration_members=66,protected_identity=False),source=source,base_source=base,
                  runtime_delta_files=['apps/platform/fixture.go'],shared_contract_files=['apps/platform/scope.json'],
                  archive_sha256=f.digest(raw),archive_bytes=len(raw),files={n:f.digest(v) for n,v in f.archive_members(raw).items()})
        write(f.SPEC,json.dumps(spec).encode());write('examples/lifecycle-d1/host.py',b'current host\n')
        write('apps/platform/repository.go',b'current product protected schema 71\n');commit()
        return spec,dict(execution='new',profile='test/v1',start='start',end='end')

    def test_full_build_uses_R_archive_and_P_host_as_distinct_facts(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'repo';spec,contract=self.fixture(root);control=Path(tmp)/'control';control.mkdir();calls=[]
            real_run=subprocess.run
            def compiler(args,**kwargs):
                if args[0]!='go':return real_run(args,**kwargs)
                cwd=Path(kwargs['cwd']);self.assertEqual(cwd,control/'build-runtime/apps/platform')
                self.assertEqual((cwd/'repository.go').read_bytes(),b'fixed schema 69\n')
                self.assertEqual(kwargs['env']['GOWORK'],'off');self.assertEqual(args[1:4],['build','-trimpath','-buildvcs=false'])
                Path(args[5]).write_bytes(b'compiled from fixed R');calls.append(kwargs['env']['GOOS'])
                return SimpleNamespace(returncode=0,stdout=b'',stderr=b'')
            with patch.object(f.subprocess,'run',compiler):a=f.build(root,control,contract)
            self.assertEqual(calls,['linux','linux','linux','windows'])
            self.assertEqual(a['source'],spec['source']);self.assertNotEqual(a['source'],a['host']['source'])
            f.verify(root,control,a,contract)
            packet=Path(tmp)/'packet';packet.mkdir()
            a['build_files']={'recorded':'b'*64}
            frozen=dict(version=2,source=a['source'],host=a['host'],runtime=spec,
                        runtime_artifact_spec_sha256=a['runtime_artifact_spec_sha256'],actual_build_source=a['source'],
                        initial_allocation_build_source=a['source'],initial_build_files=a['build_files'],
                        archive_sha256=spec['archive_sha256'],archive_bytes=spec['archive_bytes'])
            candidate=dict(Source=a['source'],Profile='test/v1',Execution='new')
            def record(value):
                (packet/'source-freeze.json').write_text(json.dumps(value))
                candidate['SourceFreezeSHA256']=f.digest((packet/'source-freeze.json').read_bytes())
            record(frozen);f.verify_frozen(packet,candidate,a,spec)
            for key,value in [('source',a['host']['source']),('host',dict(a['host'],source=a['source'])),
                              ('runtime',dict(spec,source=a['host']['source'])),('initial_build_files',{}),
                              ('runtime_artifact_spec_sha256','e'*64),('archive_sha256','f'*64),
                              ('archive_bytes',0),('version',1)]:
                bad=copy.deepcopy(frozen);bad[key]=value;record(bad)
                with self.subTest(frozen_field=key),self.assertRaises(ValueError):f.verify_frozen(packet,candidate,a,spec)
            record(frozen)
            # The fixed complete entry rejects a rehashed R/P mismatch before
            # cold topology (and therefore any Docker/source action).
            loader=importlib.util.spec_from_file_location('artifact_complete_entry',Path(__file__).with_name('verify-six-native.py'))
            v=importlib.util.module_from_spec(loader);loader.loader.exec_module(v)
            candidate.update(Profile=v.p.PROFILE,Execution=v.p.EXECUTION)
            bad=copy.deepcopy(frozen);bad['source']=a['host']['source'];record(bad)
            (packet/'candidate.json').write_text(json.dumps(candidate));(packet/'bindings.json').write_text('[]')
            with patch.object(v.p.window,'validate_packet'),patch.object(v.r,'state',return_value=a),\
                 patch.object(v.p.fixture_artifacts,'specification',return_value=spec),\
                 patch.object(v.r,'cold_topology',side_effect=AssertionError('Docker boundary crossed')):
                with self.assertRaisesRegex(ValueError,'frozen R/P'):v.sealed_gate(packet)
            cases=[('source','f'*40),('runtime_artifact_spec_sha256','f'*64),('build_environment',{'GOWORK':'auto'}),('build_cwd_relative','apps/platform')]
            for key,value in cases:
                bad=copy.deepcopy(a);bad[key]=value
                with self.subTest(field=key),self.assertRaises(ValueError):f.verify(root,control,bad,contract)
            host=root/'apps/platform/repository.go';before=host.read_bytes();host.write_bytes(b'unreviewed product change')
            with self.assertRaises(ValueError):f.verify(root,control,a,contract)
            host.write_bytes(before)
            untracked=root/'examples/lifecycle-d1/injected.py';untracked.write_bytes(b'inject')
            with self.assertRaises(ValueError):f.verify(root,control,a,contract)
            untracked.unlink();f.verify(root,control,a,contract)
            archive=control/'approved-runtime-source.zip';archive.write_bytes(archive.read_bytes()+b'trailing drift')
            with self.assertRaises(ValueError):f.verify(root,control,a,contract)

    def test_cross_artifact_contract_and_schema_refuse_before_build(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);spec,contract=self.fixture(root);path=root/f.SPEC
            changes=[('execution','old'),('profile','old/v0'),('not_before','old'),('not_after','later'),
                     ('schema',dict(max_version=71,migration_members=68,protected_identity=True)),
                     ('base_source','f'*40),('runtime_delta_files',[]),('source','f'*40)]
            for key,value in changes:
                bad=copy.deepcopy(spec);bad[key]=value;path.write_text(json.dumps(bad))
                with self.subTest(field=key),self.assertRaises((ValueError,subprocess.CalledProcessError)):f.specification(root,**contract)
            path.write_text(json.dumps(spec));scope=root/'apps/platform/scope.json';scope.write_bytes(b'foreign scope')
            with self.assertRaises(ValueError):f.specification(root,**contract)

    def test_archive_rejects_path_escape_links_duplicates_and_extra_members(self):
        for names in [['../escape'],['/absolute'],['foreign/file'],['apps/platform/a','apps/platform/a']]:
            out=io.BytesIO()
            with zipfile.ZipFile(out,'w') as z:
                for name in names:z.writestr(name,b'value')
            with self.subTest(names=names),self.assertRaises(ValueError):f.archive_members(out.getvalue())
        out=io.BytesIO()
        with zipfile.ZipFile(out,'w') as z:z.writestr('apps/platform/a',b'value')
        with self.assertRaises(ValueError):f.archive_members(out.getvalue().replace(b'apps/platform/a',b'apps\\platform\\a'))
        out=io.BytesIO()
        with zipfile.ZipFile(out,'w') as z:
            info=zipfile.ZipInfo('apps/platform/link');info.external_attr=0o120777<<16;z.writestr(info,b'/other')
        with self.assertRaises(ValueError):f.archive_members(out.getvalue())

if __name__=='__main__':unittest.main()
