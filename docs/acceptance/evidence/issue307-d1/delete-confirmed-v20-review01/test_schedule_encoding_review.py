from pathlib import Path
import hashlib,json,tomllib,unittest
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate10\authorized-execution01')
AUTOMATION=Path(r'C:\Users\Henry\.codex\automations\307-d1-candidate10-v20\automation.toml')
ORIGINAL=(CONTROL/'scheduled-prompt.txt').read_bytes()
CURRENT=tomllib.loads(AUTOMATION.read_text(encoding='utf-8'))
FAILED=json.loads((CONTROL/'failed-gate-update01.json').read_bytes())
NAME='#307 D1 candidate10/v20 \u56fa\u5b9a\u7a97\u53e3\u6267\u884c'
REPORT='88589338c265179182b561f108b4e7e7859bd8a9fecaa13db1da0afd719e3c6c'
INDEX='2775e87d5f9f5a6a6fa735c4179131399107d148948a71d33a1dbe37dd25a924'
def byte_gate(prompt):
 if not prompt.encode('utf-8').startswith(ORIGINAL):raise ValueError('original prompt not preserved')
class EncodingReview(unittest.TestCase):
 def test_actual_corrected_configuration_has_exact_original_prefix_and_name(self):
  byte_gate(CURRENT['prompt']);self.assertEqual(CURRENT['name'],NAME);self.assertEqual(hashlib.sha256(ORIGINAL).hexdigest(),'ea553adace261d0fe0cef395fe3aa17145b84f861d6454b7d2ff7fd991bf5ebd')
  self.assertEqual(CURRENT['id'],'307-d1-candidate10-v20');self.assertEqual(CURRENT['target_thread_id'],'01a13579-573d-7a32-b1e1-3297fe78380a');self.assertEqual(CURRENT['rrule'],'FREQ=DAILY;COUNT=1;BYHOUR=18;BYMINUTE=0;BYSECOND=0');self.assertEqual(CURRENT['status'],'ACTIVE');self.assertEqual(CURRENT['created_at'],FAILED['corrupted_automation']['created_at'])
 def test_actual_and_simulated_corrupted_console_prefix_are_rejected(self):
  values=[FAILED['corrupted_automation']['prompt'],ORIGINAL.decode('utf-8').encode('utf-8').decode('cp936',errors='replace')]
  for prompt in values:
   with self.subTest(actual=prompt==values[0]):
    with self.assertRaisesRegex(ValueError,'original prompt not preserved'):byte_gate(prompt)
  self.assertEqual(FAILED['status'],'FAILED_FINAL_PROMPT_BYTE_CHECK_NO_EXECUTION');self.assertTrue(FAILED['no_source_starts']);self.assertTrue(FAILED['no_permissions_enabled']);self.assertEqual(FAILED['native_delete'],0)
 def test_ascii_escaped_transport_is_encoding_independent(self):
  record={'name':NAME,'prompt':ORIGINAL.decode('utf-8')};wire=json.dumps(record,ensure_ascii=True).encode('ascii')
  for encoding in ['utf-8','cp936','cp1252']:
   with self.subTest(encoding=encoding):
    decoded=json.loads(wire.decode(encoding));self.assertEqual(decoded['name'],NAME);self.assertEqual(decoded['prompt'].encode('utf-8'),ORIGINAL);byte_gate(decoded['prompt'])
 def test_correct_suffix_only_narrows_and_preserves_original_exact_bytes(self):
  prompt=CURRENT['prompt'].encode('utf-8');suffix=prompt[len(ORIGINAL):];self.assertTrue(suffix);self.assertEqual(prompt,ORIGINAL+suffix)
  text=suffix.decode('utf-8');self.assertIn(REPORT,text);self.assertIn(INDEX,text);self.assertIn('PASS_BOUNDED_INDEPENDENT_AUTHORIZATION_WINDOW_REVIEW',text);self.assertIn('remaining_blockers=[]',text);self.assertIn('authorization_sha256',text);self.assertIn('261378648576c27a106893a2b06af425e490966495e0dfc8fccda83d64bbaab8',text);self.assertIn('\u4efb\u4e00\u65b0\u589e\u95e8\u7981\u5931\u8d25\u5219\u4e0d\u542f\u7528\u3001\u4e0d\u53d1\u9001',text);self.assertNotIn('\ufffd',CURRENT['prompt'])
if __name__=='__main__':unittest.main(verbosity=2)