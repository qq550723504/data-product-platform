"""Actual Git blob/archive/checkout preservation; no Docker or provider."""
import hashlib
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

ROOT=Path(__file__).resolve().parents[2]
RULE='docs/acceptance/evidence/issue307-d1/** -text whitespace=cr-at-eol,-blank-at-eol,-blank-at-eof,-space-before-tab'
MARKDOWN=[f'docs/acceptance/issue307-d1-v29-{suffix}.md' for suffix in ('crosscheck01','execution01','schedule01','window01')]
MARKDOWN.append('docs/acceptance/issue307-d1-v31-preparation.md')
CURRENT_DOCUMENTS=('issue307-d1-candidate22-confirmation.md','issue307-d1-candidate22-final-confirmation01.md',
                   'issue307-d1-v32-window-runbook.md','issue307-d1-v32-preparation-contract03.md',
                   'issue307-d1-v32-preparation.md')


class ReceiptBytesTests(unittest.TestCase):
    def git(self,root,*args,autocrlf='true'):
        return subprocess.check_output(['git','-c','core.longpaths=true','-c','core.autocrlf='+autocrlf,*args],cwd=root,stderr=subprocess.PIPE)

    def fixture(self,folder,*,current):
        repo=folder/'source';repo.mkdir();self.git(repo,'init','-q')
        attributes=(ROOT/'.gitattributes').read_text(encoding='utf-8')
        self.assertIn(RULE,attributes)
        rules=[RULE,*[name+' -text' for name in MARKDOWN]]
        for rule in rules:self.assertIn(rule,attributes)
        if not current:
            for rule in rules:attributes=attributes.replace(rule,'')
        (repo/'.gitattributes').write_text(attributes,encoding='utf-8',newline='\n')
        child_attributes=(ROOT/'docs/acceptance/.gitattributes').read_text(encoding='utf-8')
        for name in CURRENT_DOCUMENTS:self.assertIn(name+' -text',child_attributes)
        if not current:
            for name in CURRENT_DOCUMENTS:child_attributes=child_attributes.replace(name+' -text','')
        files={
            'docs/acceptance/.gitattributes':child_attributes.encode('utf-8'),
            'docs/acceptance/evidence/issue307-d1/delete-confirmed-v29-execution01/window01/terminal.json':b'{"status":"FAILED"}\r\n',
            'docs/acceptance/evidence/issue307-d1/candidate21-planning01/process.stderr.txt':b'original error\r\n\r\n',
            'docs/acceptance/evidence/issue307-d1/execution-freeze-review01/index.json':b'{"count":1}\r\n',
            'docs/acceptance/evidence/issue307-d1/delete-confirmed-v31-execution01/user.txt':'授权\n'.encode('utf-8'),
        }
        files.update({name:(ROOT/name).read_bytes() for name in MARKDOWN})
        # Current documents are rendered after native acceptance. Exercise the
        # already committed exact-path rules using both native line endings.
        files.update({'docs/acceptance/'+name:('精确确认\n' if n%2==0 else '原始回执\r\n').encode('utf-8')
                      for n,name in enumerate(CURRENT_DOCUMENTS)})
        for name,raw in files.items():
            path=repo/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(raw)
        self.git(repo,'add','.')
        self.git(repo,'-c','user.name=Receipt Test','-c','user.email=receipt@example.invalid','commit','-qm','original receipt bytes')
        return repo,files

    def test_current_rule_keeps_original_blob_archive_and_both_checkout_modes(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder=Path(tmp);repo,files=self.fixture(folder,current=True)
            with zipfile.ZipFile(io.BytesIO(self.git(repo,'archive','--format=zip','HEAD'))) as archive:
                for name,raw in files.items():
                    self.assertEqual(self.git(repo,'show','HEAD:'+name),raw)
                    self.assertEqual(archive.read(name),raw)
            for mode in ('true','false'):
                checkout=folder/('checkout-'+mode)
                self.git(repo,'clone','--no-local','-q',str(repo),str(checkout),autocrlf=mode)
                for name,raw in files.items():self.assertEqual((checkout/name).read_bytes(),raw)

    def test_prior_normalizing_rule_breaks_frozen_raw_sha_even_if_text_equal(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo,files=self.fixture(Path(tmp),current=False)
            name='docs/acceptance/evidence/issue307-d1/delete-confirmed-v29-execution01/window01/terminal.json'
            raw=files[name];blob=self.git(repo,'show','HEAD:'+name)
            self.assertEqual(blob.replace(b'\r\n',b'\n'),raw.replace(b'\r\n',b'\n'))
            self.assertNotEqual(hashlib.sha256(blob).hexdigest(),hashlib.sha256(raw).hexdigest())

    def test_prior_rule_changes_indexed_markdown_outside_evidence_on_checkout(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder=Path(tmp);repo,files=self.fixture(folder,current=False)
            checkout=folder/'checkout'
            self.git(repo,'clone','--no-local','-q',str(repo),str(checkout),autocrlf='true')
            for name in MARKDOWN:
                raw=files[name];observed=(checkout/name).read_bytes()
                self.assertEqual(observed.replace(b'\r\n',b'\n'),raw.replace(b'\r\n',b'\n'))
                self.assertNotEqual(hashlib.sha256(observed).hexdigest(),hashlib.sha256(raw).hexdigest())


if __name__=='__main__':unittest.main()
