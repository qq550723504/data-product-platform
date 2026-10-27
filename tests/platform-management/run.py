"""Own one complete acceptance suite, then stop it while preserving its data."""
import argparse
import importlib
import json
import os
from pathlib import Path
import signal
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tests'))
from acceptance_lifecycle import AcceptanceRun, CleanupError, finish, load_module

MODES = {
    'readonly': None,
    'runtime': ('MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK', 'ISOLATED_RUNTIME_LOG_READONLY'),
    'main-service': ('MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK', 'ISOLATED_MAIN_SERVICE_READONLY'),
    'compat': ('MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK', 'ISOLATED_MAIN_SERVICE_COMPAT_READONLY'),
    'directory': ('MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK', 'ISOLATED_DIRECTORY_PRODUCT_READONLY'),
    'user-product': ('MANAGEMENT_USER_PRODUCT_FIXTURE_ACK', 'ISOLATED_USER_PRODUCT_READONLY'),
}
DIST = {'readonly': '20261025', 'runtime': 'runtime-logs-20261026', 'main-service': 'main-service-20261026',
        'compat': 'main-service-compat-20261026', 'directory': 'directory-product-20261026',
        'user-product': 'user-product-20261027'}


def configure(suite):
    if os.environ.get('MANAGEMENT_FIXTURE_ACK') != 'ISOLATED_READONLY_MANAGEMENT':
        raise CleanupError('EXPLICIT_INDEPENDENT_ACCEPTANCE_REQUIRED')
    selected = MODES[suite]
    for mode in MODES.values():
        if mode and os.environ.get(mode[0]) and (mode != selected or os.environ[mode[0]] != mode[1]):
            raise CleanupError('CONFLICTING_ACCEPTANCE_MODE')
    if selected:
        os.environ[selected[0]] = selected[1]
    os.environ.setdefault('MANAGEMENT_WEB_DIST', '.next-acceptance-identity-management-' + DIST[suite])


def stages(suite):
    before = []
    if suite in ('main-service', 'compat', 'directory', 'user-product'):
        before.append('test-main-service.py')
    if suite in ('directory', 'user-product'):
        before.append('directory-product-contract.py')
    browser = {'readonly': ['platform-management/browser.mjs'],
               'runtime': ['runtime-logs/api-browser.mjs', 'platform-management/browser.mjs'],
               'main-service': ['platform-management/main-service-browser.mjs'],
               'compat': ['platform-management/compat-boundaries.mjs', 'platform-management/browser.mjs', 'runtime-logs/api-browser.mjs'],
               'directory': ['platform-management/directory-product-browser.mjs'],
               'user-product': ['platform-management/user-product-browser.mjs']}[suite]
    after = ['probe-preservation.py'] if suite == 'readonly' else []
    if suite in ('main-service', 'compat', 'directory', 'user-product'):
        after.append('verify-main-service.py')
    return before, browser, after


def execute(suite, b, scope):
    stage = 'bootstrap'
    status = 'FAIL'
    try:
        b.main(lifecycle=scope)
        before, browser, after = stages(suite)
        for filename in before:
            stage = filename
            b.call([sys.executable, str(Path(__file__).parent / filename)], timeout=300)
        stage = 'start-web'
        load_module('_managed_acceptance_web', Path(__file__).with_name('start-web.py')).main(build=True)
        if suite == 'runtime':
            os.environ.setdefault('MANAGEMENT_BROWSER_OUTPUT', 'management-readonly-runtime-review-20261026')
        for filename in browser:
            stage = filename
            b.call(['node', str(ROOT / 'tests' / filename)], timeout=900)
        for filename in after:
            stage = filename
            b.call([sys.executable, str(Path(__file__).parent / filename)], timeout=180)
        status = 'PASS'
    finally:
        if scope.started:
            with (b.RUN / 'acceptance-result.json').open('x', encoding='utf-8') as stream:
                json.dump({'suite': suite, 'status': status, 'lastStage': stage,
                           'shutdownEvidence': 'cleanup-*.json', 'evidenceRetained': True}, stream, indent=2)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--suite', required=True, choices=MODES)
    args = parser.parse_args()
    configure(args.suite)
    b = importlib.import_module('bootstrap')
    scope = AcceptanceRun(b.ROOT, b.RUN)
    def interrupted(signum, frame):
        raise KeyboardInterrupt
    previous = signal.signal(signal.SIGTERM, interrupted)
    try:
        finish(scope, lambda: execute(args.suite, b, scope))
    finally:
        signal.signal(signal.SIGTERM, previous)


if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt):
        print('FAIL acceptance or shutdown; inspect retained acceptance-result.json and cleanup-*.json', file=sys.stderr)
        sys.exit(1)
