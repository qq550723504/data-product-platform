import io
import tarfile
import unittest
import retired_budget_bytes as b

def archive(prefix,body=b'used UNKNOWN fact'):
    stream=io.BytesIO()
    with tarfile.open(fileobj=stream,mode='w') as out:
        root=tarfile.TarInfo(prefix);root.type=tarfile.DIRTYPE;root.uid=root.gid=10001;root.mode=0o755;out.addfile(root)
        entry=tarfile.TarInfo(prefix+'/physical-attempt');entry.size=len(body);entry.uid=entry.gid=10001;entry.mode=0o600;out.addfile(entry,io.BytesIO(body))
    return stream.getvalue()

class PhysicalBudgetTests(unittest.TestCase):
    def test_equivalent_tar_headers_preserve_exact_file_and_owner_facts(self):
        self.assertEqual(b.contents(archive('budget'),docker_copy=True),b.contents(archive('.'),docker_copy=False))

    def test_reset_or_changed_used_budget_is_visible(self):
        old=b.contents(archive('budget'),docker_copy=True)
        for value in (b'',b'changed'):
            self.assertNotEqual(old,b.contents(archive('.',value),docker_copy=False))

    def test_wrong_root_type_paths_reject(self):
        with self.assertRaises(ValueError):b.contents(archive('other'),docker_copy=True)
        with self.assertRaises(ValueError):b.contents(archive('../unsafe'),docker_copy=False)
        with self.assertRaises(ValueError):b.contents('str',docker_copy=True)

if __name__=='__main__':unittest.main()
