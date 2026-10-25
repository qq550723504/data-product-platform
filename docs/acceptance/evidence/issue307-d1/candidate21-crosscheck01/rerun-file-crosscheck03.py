"""Use the actual frozen operator/root permission schemas, preserving 01/02."""
from pathlib import Path

original=Path(__file__).with_name('crosscheck02.py')
source=original.read_text(encoding='utf-8')
old="            need(load(owner/'permission.json')=={'Enabled':False} and not (owner/'human-confirmation.txt').exists(), 'Permission enabled or human text materialized')"
new="""            expected=dict(Enabled=False,Execution=EXECUTION,CandidateSHA256=sha(PACKET/'candidate.json'),ConfirmationSHA256=sha(PACKET/'confirmation-template.txt'),NotBefore=c['NotBefore'],ExpiresAt=c['NotAfter'],ConfirmedAt='0001-01-01T00:00:00Z',MaxPerTarget=1,MaxTotal=6)
            if not stage.startswith('root-'): expected.update(Profile=c['Profile'],BinarySHA256=c['SenderBinarySHA256'])
            need(load(owner/'permission.json')==expected and not (owner/'human-confirmation.txt').exists(), 'Actual frozen operator/root permission or materialized authority differs')"""
if source.count(old)!=1:
    raise ValueError('Original 02 checker shape differs')
source=source.replace(old,new)
needle="folder=OUT/(args.mode+'02');folder.mkdir(exist_ok=False)"
if source.count(needle)!=1:raise ValueError('Original 02 output differs')
source=source.replace(needle,"folder=OUT/(args.mode+'03');folder.mkdir(exist_ok=False)")
corrected=original.with_name('crosscheck03.py')
with corrected.open('xb') as stream:stream.write(source.encode('utf-8'))
exec(compile(source,str(corrected),'exec'),{'__file__':str(corrected.resolve()),'__name__':'__main__'})
