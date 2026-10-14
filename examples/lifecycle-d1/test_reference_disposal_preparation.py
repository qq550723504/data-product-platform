import importlib.util, sys, unittest, tempfile, json
from pathlib import Path
from unittest.mock import patch
from types import SimpleNamespace

spec=importlib.util.spec_from_file_location('reference_disposal',Path(__file__).with_name('prepare-reference-disposal.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class PreparationSafety(unittest.TestCase):
    def test_destructive_and_expanded_arguments_fail_before_any_io(self):
        for args in [('execute',),('provision',),('enable',),('cleanup',),('native-before','--human-confirmed'),('native-before','--target','foreign'),('no-send','--release','foreign')]:
            with self.subTest(args=args),patch.object(sys,'argv',['prepare',*args]),patch.object(module.subprocess,'run') as run:
                with self.assertRaises(SystemExit):module.main()
                run.assert_not_called()
    def test_authorization_check_survives_optimized_python(self):
        with self.assertRaises(RuntimeError):module.require(False,'unsupported authority')
    def test_frozen_source_topology_input_cannot_be_redefined_by_working_files(self):
        with patch.object(module,'call',side_effect=RuntimeError('pinned input unavailable')) as call,patch.object(module,'docker') as docker:
            with self.assertRaises(RuntimeError):module.preflight()
            self.assertEqual(call.call_args.args[0][0:3],['git','show','917465d88a0f6e8dfd0a4745fb4d8d767a2c75a1:docs/acceptance/evidence/issue307-d1/delete-proposal-5d2274ee/topology-readback.json'])
            docker.assert_not_called()
    def test_render_keeps_strict_config_path_and_private_output_mount(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary);control=root/'private';public=root/'public';control.mkdir();public.mkdir();(control/'bin').mkdir();(control/'bin/executor').write_bytes(b'fixture')
            cfg={key:'fixture' for key in ('RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','PostgresDSN','InspectAccess','InspectSecret','Token')}
            commands=[]
            def docker(*args):
                commands.append(args)
                if args[0]=='wait':
                    payload=control/'render/payload'
                    for name in ('bounded-profile.json','delete-executor-policy.json'): (payload/name).write_text('{}')
                    (payload/'gateway-active.Caddyfile').write_text('private-inspector-value')
                    return b'0'
                return b''
            with patch.object(module,'CONTROL',control),patch.object(module,'PUBLIC',public),patch.object(module,'preflight',return_value=cfg),patch.object(module,'docker',side_effect=docker),patch.object(module,'inspect',return_value={'Id':'fixture','Image':'fixture','HostConfig':{'NetworkMode':'none'}}),patch.object(module.subprocess,'run',return_value=SimpleNamespace(stdout=b'',stderr=b'')):
                module.helper('render','fixture',mode='render')
            config=json.loads((control/'render/reference.json').read_text())
            self.assertEqual(config['Fixture']['OutputDirectory'],'/run/output')
            create=commands[0]
            self.assertIn('type=bind,source='+str(control/'render/payload')+',target=/run/output',create)
            self.assertNotIn('type=bind,source='+str(public/'render')+',target=/run/output',create)
            self.assertEqual(set(p.name for p in (public/'render').iterdir()),{'bounded-profile.json','delete-executor-policy.json','helper.json'})
if __name__=='__main__':unittest.main()
