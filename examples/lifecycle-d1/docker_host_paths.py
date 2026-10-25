"""One host-filesystem boundary for Docker inspect bind-mount sources."""
import os
from pathlib import Path, PureWindowsPath
import re
from evidence_indexes import unlinked_absolute


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


def _regular_host_file(value):
    """Inspect original components before resolving an existing local file."""
    if type(value) is not str or not value or '\x00' in value:
        raise ValueError('Docker bind file must be a nonempty text path')
    if os.name == 'nt':
        lexical = PureWindowsPath(value)
        # This boundary is for drive-backed Desktop files, not UNC shares,
        # device paths, ADS, drive-relative paths or invented VM aliases.
        if not re.fullmatch('[a-zA-Z]:', lexical.drive) or not lexical.root:
            raise ValueError('Docker bind file must have an exact local drive')
        components = value.replace('\\', '/')[3:].split('/')
        if any(':' in part or part != part.rstrip(' .') for part in components):
            raise ValueError('Docker bind file has an ambiguous Windows component')
    else:
        if not value.startswith('/'):
            raise ValueError('Docker bind file must be absolute on this host')
        components = value[1:].split('/')
    if any(part in ('', '.', '..') for part in components):
        raise ValueError('Docker bind file has an empty or traversing component')
    path = unlinked_absolute(value)
    if not path.is_file() or path.stat().st_nlink != 1:
        raise ValueError('Docker bind file must be one regular unlinked file')
    return path


def bound_file_source(value, expected):
    """Map a Desktop source, then require the same unlinked physical file.

    Conversion is namespace decoding only. It never substitutes a digest for
    identity, resolves away a junction/symlink, or accepts another mount set.
    Native Linux sources retain their existing absolute-path representation.
    """
    if type(value) is not str or not value or '\x00' in value:
        raise ValueError('Docker mount source must be a nonempty text path')
    prefix = '/run/desktop/mnt/host/'
    if value.startswith(prefix):
        if os.name != 'nt' or '\\' in value:
            raise ValueError('Docker Desktop drive source requires a Windows host')
    actual = _regular_host_file(normalize_source(value))
    wanted = _regular_host_file(os.fspath(expected))
    if not actual.samefile(wanted):
        raise ValueError('Docker bind source is not the approved physical file')
    return actual
