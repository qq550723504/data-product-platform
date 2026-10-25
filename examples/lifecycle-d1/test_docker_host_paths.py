"""Real temporary file identities only; no Docker/provider/retained access."""
import ctypes
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from docker_host_paths import bound_file_source


def desktop(path):
    value=Path(path).as_posix()
    return '/run/desktop/mnt/host/'+value[0].lower()+value[2:]


class BoundFileTests(unittest.TestCase):
    def test_actual_native_regular_file_identity(self):
        with tempfile.TemporaryDirectory() as d:
            path=Path(d).resolve()/'one file';path.write_bytes(b'trusted content')
            self.assertTrue(bound_file_source(str(path),path).samefile(path))

    @unittest.skipUnless(os.name=='nt','Windows Desktop namespace requires native Windows')
    def test_actual_desktop_drive_and_native_aliases_have_one_file_identity(self):
        with tempfile.TemporaryDirectory(prefix='bind identity ') as d:
            path=Path(d).resolve()/'one file';path.write_bytes(b'trusted content')
            short=ctypes.windll.kernel32.GetShortPathNameW
            short.argtypes=[ctypes.c_wchar_p,ctypes.c_wchar_p,ctypes.c_uint];short.restype=ctypes.c_uint
            buf=ctypes.create_unicode_buffer(32768);size=short(str(path),buf,len(buf))
            self.assertTrue(0<size<len(buf))
            for value in (str(path),str(path).upper(),buf.value,desktop(path),desktop(Path(str(path).upper())),desktop(Path(buf.value))):
                with self.subTest(source=value):self.assertTrue(bound_file_source(value,path).samefile(path))

    def test_identical_bytes_are_not_physical_file_identity(self):
        with tempfile.TemporaryDirectory() as d:
            expected=Path(d)/'expected';expected.write_bytes(b'same')
            other=Path(d)/'other';other.write_bytes(expected.read_bytes())
            with self.assertRaisesRegex(ValueError,'approved physical file'):bound_file_source(str(other),expected)

    def test_hardlink_to_approved_file_is_rejected_on_both_sides(self):
        with tempfile.TemporaryDirectory() as d:
            expected=Path(d)/'expected';expected.write_bytes(b'same')
            other=Path(d)/'hardlink';os.link(expected,other)
            for source,wanted in ((str(expected),expected),(str(other),expected),(str(expected),other)):
                with self.subTest(source=source,wanted=wanted),self.assertRaises(ValueError):bound_file_source(source,wanted)

    def test_symlink_in_original_file_or_parent_is_not_resolved_away(self):
        with tempfile.TemporaryDirectory() as d:
            folder=Path(d).resolve();target=folder/'actual';target.mkdir();binary=target/'binary';binary.write_bytes(b'bound')
            for name,point,directory in (('file-link',binary,False),('dir-link',target,True)):
                link=folder/name
                try:link.symlink_to(point,target_is_directory=directory)
                except OSError as error:
                    if os.name!='nt':raise
                    # Directory junction coverage is a separate Windows test;
                    # a host without symlink privilege must not fake a link.
                    self.skipTest('Windows symlink creation unavailable: '+str(error))
                try:
                    alias=link/'binary' if directory else link
                    for source,wanted in ((str(alias),binary),(str(binary),alias)):
                        with self.subTest(name=name,source=source),self.assertRaises(ValueError):bound_file_source(source,wanted)
                finally:link.unlink()

    @unittest.skipUnless(os.name=='nt','NTFS junction is a Windows boundary')
    def test_actual_junction_source_and_expected_rejected_before_resolution(self):
        import base64,subprocess
        with tempfile.TemporaryDirectory() as d:
            folder=Path(d).resolve();actual=folder/'actual';actual.mkdir();path=actual/'binary';path.write_bytes(b'bound');alias=folder/'alias'
            # Native .NET link creation is unnecessary; reuse PowerShell's
            # Junction command with an encoded script, no shell interpolation.
            script="$ErrorActionPreference='Stop'; New-Item -ItemType Junction -Path '"+str(alias).replace("'","''")+"' -Target '"+str(actual).replace("'","''")+"' | Out-Null"
            result=subprocess.run(['pwsh','-NoProfile','-NonInteractive','-EncodedCommand',base64.b64encode(script.encode('utf-16le')).decode()],capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,result.stderr.decode('utf8','replace'));self.assertTrue(alias.is_junction())
            try:
                for source,wanted in ((str(alias/'binary'),path),(desktop(alias/'binary'),path),(str(path),alias/'binary')):
                    with self.subTest(source=source),self.assertRaises(ValueError):bound_file_source(source,wanted)
            finally:alias.rmdir()

    def test_lexical_namespace_and_type_counterexamples(self):
        with tempfile.TemporaryDirectory() as d:
            expected=Path(d).resolve()/'expected';expected.write_bytes(b'trusted')
            bad=[None,True,b'/tmp/binary',expected,'','relative/binary','x\x00y',str(expected.parent)+'/./expected',str(expected.parent)+'/other/../expected',
                 str(expected.parent)+'//expected','/run/desktop/mnt/host/cc/binary','/run/desktop/mnt/host/c/../binary',
                 '/run/desktop/mnt/host/c//binary','/run/desktop/mnt/host/c/./binary','/run/desktop/mnt/host/c/a\\binary',
                 '/run/desktop/mnt/host/c/a:stream','/run/desktop/mnt/host/c/a.','/run/desktop/mnt/host/c/a ',
                 '/host_mnt/c/binary','/mnt/c/binary','//run/desktop/mnt/host/c/binary']
            for source in bad:
                with self.subTest(source=source),self.assertRaises((ValueError,OSError)):bound_file_source(source,expected)
            if os.name=='nt':
                for source in ('C:relative','\\\\?\\'+str(expected),'\\\\server\\share\\binary',str(expected)+':stream'):
                    with self.subTest(source=source),self.assertRaises(ValueError):bound_file_source(source,expected)

    def test_missing_directory_or_expected_nonfile_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            path=Path(d).resolve()/'actual';path.write_bytes(b'bound')
            for source,wanted in ((str(path.parent),path),(str(path),path.parent),(str(path.parent/'missing'),path)):
                with self.subTest(source=source,wanted=wanted),self.assertRaises((ValueError,OSError)):bound_file_source(source,wanted)

    @unittest.skipIf(os.name=='nt','native Linux cannot borrow a Windows Desktop drive source')
    def test_linux_rejects_windows_namespace_even_if_projection_is_faked(self):
        with tempfile.TemporaryDirectory() as d:
            path=Path(d).resolve()/'actual';path.write_bytes(b'bound')
            with patch('docker_host_paths.normalize_source',return_value=str(path)),self.assertRaises(ValueError):
                bound_file_source('/run/desktop/mnt/host/c/actual',path)


if __name__=='__main__':unittest.main()
