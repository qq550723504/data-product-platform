"""One host-filesystem boundary for Docker inspect bind-mount sources."""
from pathlib import Path
import re


def normalize_source(value):
    if not isinstance(value, str) or not value or '\x00' in value:
        raise ValueError('Docker mount source must be a nonempty text path')
    prefix = '/run/desktop/mnt/host/'
    if value.startswith(prefix):
        tail = value[len(prefix):]
        if not re.fullmatch(r'[a-zA-Z]/[^\x00]*', tail):
            raise ValueError('Docker Desktop mount source has no exact drive')
        value = tail[0].upper() + ':/' + tail[2:]
    return value


def hostpath(value):
    path = Path(normalize_source(value))
    if not path.is_absolute():
        raise ValueError('Docker mount source must be absolute on this host')
    return path.resolve()
