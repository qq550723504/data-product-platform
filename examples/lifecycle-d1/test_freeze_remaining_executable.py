"""Offline bounded preparation-close failure regression. No Docker/secrets."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('freeze',Path(__file__).with_name('freeze-remaining-executable.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f)

class PreparationClosureTests(unittest.TestCase):
    def test_stop_or_inspect_failure_continues_every_other_owner(self):
        stopped=[]
        def inspect(ident):
            if ident=='inspect-fails':raise RuntimeError('synthetic read failure')
            return {'State':{'Running':ident not in stopped,'Pid':0 if ident in stopped else 1},
                    'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}
        def docker(*args):
            if args[-1]=='stop-fails':raise RuntimeError('synthetic stop failure')
            stopped.append(args[-1])
        with patch.object(f.r,'inspect',inspect),patch.object(f.r,'docker',docker):
            result=f.close_known(['last-good','inspect-fails','stop-fails'])
        self.assertFalse(result['all_closed'])
        self.assertEqual([r['id'] for r in result['containers']],['stop-fails','inspect-fails','last-good'])
        self.assertEqual(stopped,['last-good'])
        self.assertTrue(result['containers'][-1]['stopped_pid_zero'])
        self.assertEqual(result['cleanup'],0)

if __name__=='__main__':unittest.main(verbosity=2)
