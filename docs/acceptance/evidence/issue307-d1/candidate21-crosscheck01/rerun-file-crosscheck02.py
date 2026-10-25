"""Retain the original checker failure; correct only its row-field offsets."""
from pathlib import Path

original = Path(__file__).with_name('crosscheck.py')
source = original.read_text(encoding='utf-8')
changes = [
    ('parts[1:6]', 'parts[1:5]'),
    ('parts[6:9]', 'parts[5:8]'),
    ('parts[9].startswith', 'parts[8].startswith'),
    ('pid=parts[9][5:]; phash=parts[10][5:]', 'pid=parts[8][5:]; phash=parts[9][5:]'),
    ('folder=OUT/args.mode;folder.mkdir(exist_ok=False)',
     "folder=OUT/(args.mode+'02');folder.mkdir(exist_ok=False)"),
]
for before, after in changes:
    if source.count(before) != 1:
        raise ValueError('Original crosschecker source differs: ' + before)
    source = source.replace(before, after)
corrected = original.with_name('crosscheck02.py')
with corrected.open('xb') as stream:
    stream.write(source.encode('utf-8'))
exec(compile(source, str(corrected), 'exec'), {'__file__': str(corrected.resolve()), '__name__': '__main__'})
