import copy
import io
from pathlib import Path
import tarfile
import tempfile
import unittest
import retention_staging as s

class StagingTests(unittest.TestCase):
    def metadata(self):
        return dict(Name='d1-retention-private-test',Driver='local',Scope='local',Options=None,
                    Labels={'dpp.retention-maintenance':'preparation-only'},Mountpoint='/var/lib/docker/volumes/d1-retention-private-test/_data')

    def archive(self,*,uid=10001,mode=0o600,link=False):
        out=io.BytesIO()
        with tarfile.open(fileobj=out,mode='w') as tar:
            root=tarfile.TarInfo('.');root.type=tarfile.DIRTYPE;root.uid=root.gid=10001;root.mode=0o700;tar.addfile(root)
            f=tarfile.TarInfo('./control.json');f.uid=uid;f.gid=10001;f.mode=mode;f.size=2
            if link:f.type=tarfile.LNKTYPE;f.linkname='/outside';f.size=0
            tar.addfile(f,None if link else io.BytesIO(b'{}'))
        return out.getvalue()

    def test_explicit_local_volume_mapping_and_metadata_counterexamples(self):
        m=self.metadata();linux,windows=s.paths(m)
        self.assertEqual(linux,'/mnt/docker-desktop-disk/data/docker/volumes/'+m['Name']+'/_data')
        self.assertEqual(windows,'\\\\wsl.localhost\\docker-desktop'+linux.replace('/','\\'))
        for key,value in [('Driver','nfs'),('Scope','global'),('Options',{'device':'/old'}),('Labels',{}),('Mountpoint','/elsewhere'),('Name','../old')]:
            bad=copy.deepcopy(m);bad[key]=value
            with self.subTest(key=key),self.assertRaises(ValueError):s.paths(bad)

    def test_complete_snapshot_and_owner_mode_hardlink_byte_type_denials(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);(root/'control.json').write_bytes(b'{}')
            for fault in ['none','uid','mode','link','outside-hardlink','bytes','type','extra']:
                raw=self.archive(uid=0 if fault=='uid' else 10001,mode=0o644 if fault=='mode' else 0o600,link=fault=='link')
                if fault=='type':raw='not bytes'
                if fault=='bytes':(root/'control.json').write_bytes(b'changed')
                if fault=='extra':(root/'extra').write_bytes(b'x')
                def call(*args):
                    self.assertEqual(args[:3],('stat','-c','%u:%g:%a:%h:%F'))
                    return b'10001:10001:600:'+ (b'2' if fault=='outside-hardlink' else b'1') + b':regular file\n'
                if fault=='none':self.assertEqual(len(s.validate_snapshot(root,'/exact',raw,files={'control.json'},call=call)),2)
                else:
                    with self.subTest(fault=fault),self.assertRaises(ValueError):s.validate_snapshot(root,'/exact',raw,files={'control.json'},call=call)
                (root/'control.json').write_bytes(b'{}')
                if fault=='extra':(root/'extra').unlink()

if __name__=='__main__':unittest.main()
