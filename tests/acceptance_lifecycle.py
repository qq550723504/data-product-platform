"""Run-scoped acceptance shutdown using Docker and the existing trial PID guard.

This owns only resources registered at creation. It never adopts old fixtures,
enumerates stop targets, removes containers, or deletes persistent data.
"""
from contextlib import contextmanager
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import select
import signal
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
LABEL = 'org.dpp.acceptance.run'
SCHEMA = 'acceptance-lifecycle.v1'


def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


trial = load_module('_acceptance_trial_runtime', ROOT / 'deploy/trial/runtime.py')
helpers = load_module('_acceptance_helper_files', ROOT / 'examples/temporary_containers.py')


class CleanupError(RuntimeError):
    pass


def need(condition, code):
    if not condition:
        raise CleanupError(code)


def sha(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def unlinked(path):
    for p in (path, *path.parents):
        need(not p.is_symlink() and not getattr(p, 'is_junction', lambda: False)(), 'LINKED_CONTROL_PATH')


def process_identity(pid, *, expected=None, terminate=False):
    """Pin the native process object when signaling, never kill a PID tree."""
    if os.name == 'nt':
        try:
            return trial.process_identity(pid, expected=expected, terminate=terminate, allow_missing=True)
        except trial.Refused:
            raise CleanupError('PROCESS_IDENTITY_UNVERIFIED') from None
    need(sys.platform == 'linux' and hasattr(os, 'pidfd_open') and hasattr(signal, 'pidfd_send_signal'),
         'SAFE_PROCESS_HANDLES_UNAVAILABLE')
    try:
        fd = os.pidfd_open(pid)
    except ProcessLookupError:
        return None
    try:
        poll = select.poll()
        poll.register(fd, select.POLLIN)
        if poll.poll(0):
            return None
        try:
            stat = Path(f'/proc/{pid}/stat').read_text()
            value = {'pid': pid, 'created': int(stat[stat.rfind(')') + 2:].split()[19]),
                     'exe': os.readlink(f'/proc/{pid}/exe')}
        except (FileNotFoundError, ProcessLookupError):
            if poll.poll(0):
                return None
            raise CleanupError('PROCESS_IDENTITY_UNVERIFIED') from None
        need(expected is None or value == expected, 'PROCESS_IDENTITY_UNVERIFIED')
        if terminate:
            try:
                signal.pidfd_send_signal(fd, signal.SIGTERM)
            except ProcessLookupError:
                return None
            need(bool(poll.poll(10000)), 'PROCESS_STOP_UNCONFIRMED')
        return value
    finally:
        os.close(fd)


def environment():
    value = trial.environment()
    if os.name != 'nt':
        value['HOME'] = str(Path.home())  # Docker's per-user context directory on Linux.
    return value


def command(args, **kwargs):
    try:
        result = subprocess.run([str(a) for a in args], capture_output=True,
                                env=environment(), timeout=60, **kwargs)
    except (OSError, subprocess.TimeoutExpired):
        raise CleanupError('RUNTIME_OBSERVATION_UNAVAILABLE') from None
    need(result.returncode == 0, 'RUNTIME_COMMAND_FAILED')
    return result.stdout.decode('utf-8').strip()


def container_identity(item):
    # Docker is free to reorder mounts; reuse the preservation contract's rule.
    host = dict(item['HostConfig'])
    # Docker Desktop changes this one disabled default from false to null on
    # start. True, missing fields, and every other field/type remain distinct.
    if 'OomKillDisable' in host and host['OomKillDisable'] is None:
        host['OomKillDisable'] = False
    return sha({'id': item['Id'], 'name': item['Name'], 'image': item['Image'],
                'created': item['Created'], 'config': item['Config'], 'host': host,
                'mounts': sorted(item['Mounts'], key=lambda m: json.dumps(m, sort_keys=True))})


@contextmanager
def locked(path):
    with path.open('a+b') as stream:
        stream.seek(0)
        stream.write(b'0')
        stream.flush()
        stream.seek(0)
        try:
            if os.name == 'nt':
                import msvcrt
                msvcrt.locking(stream.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(stream.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            raise CleanupError('RUN_OWNED_BY_ANOTHER_SESSION') from None
        try:
            yield
        finally:
            stream.seek(0)
            if os.name == 'nt':
                msvcrt.locking(stream.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                fcntl.flock(stream.fileno(), fcntl.LOCK_UN)


class AcceptanceRun:
    def __init__(self, root, run, *, execute=command, process=process_identity):
        self.root, self.run = Path(root).absolute(), Path(run).absolute()
        need(self.run.parent == self.root / '.data', 'FOREIGN_RUN_DIRECTORY')
        unlinked(self.run)
        self.execute, self.process = execute, process
        self.state, self.control, self.guard = None, None, None
        self.children = []

    @property
    def started(self):
        return self.state is not None

    @staticmethod
    def private_base():
        return (Path(os.environ['LOCALAPPDATA']) / 'Codex/acceptance-runs' if os.name == 'nt'
                else Path.home() / '.local/state/dpp/acceptance-runs')

    def begin(self):
        need(not self.run.exists(), 'EXISTING_RUN_RETAINED')
        context = self.execute(['docker', 'context', 'show'])
        need(bool(context) and '\n' not in context, 'DOCKER_CONTEXT_UNVERIFIED')
        daemon = self.execute(['docker', '--context', context, 'info', '--format', '{{.ID}}'])
        need(bool(daemon), 'DOCKER_DAEMON_UNVERIFIED')
        run_id = str(uuid.uuid4())
        self.control = helpers.private_root(self.private_base() / run_id)
        self.guard = locked(self.control / 'owner.lock')
        self.guard.__enter__()
        self.run.mkdir(mode=0o700, parents=True)
        locator = {'schemaVersion': SCHEMA, 'runId': run_id, 'rootHash': sha(str(self.root))}
        with (self.run / 'lifecycle.json').open('x', encoding='utf-8') as f:
            json.dump(locator, f)
        self.state = {**locator, 'run': str(self.run), 'context': context, 'daemon': daemon,
                      'containers': [], 'processes': []}
        self.save()

    @classmethod
    def reopen(cls, root, run):
        result = cls(root, run)
        locator_path = result.run / 'lifecycle.json'
        unlinked(locator_path)
        locator = json.loads(locator_path.read_text(encoding='utf-8'))
        need(set(locator) == {'schemaVersion', 'runId', 'rootHash'} and locator['schemaVersion'] == SCHEMA
             and locator['rootHash'] == sha(str(result.root)), 'FOREIGN_RUN_DIRECTORY')
        need(str(uuid.UUID(locator['runId'])) == locator['runId'], 'INVALID_RUN_ID')
        result.control = result.private_base() / locator['runId']
        unlinked(result.control / 'state.json')
        result.guard = locked(result.control / 'owner.lock')
        result.guard.__enter__()
        try:
            result.state = json.loads((result.control / 'state.json').read_text(encoding='utf-8'))
            need(all(result.state.get(k) == v for k, v in locator.items()) and result.state['run'] == str(result.run),
                 'FOREIGN_RUN_DIRECTORY')
        except BaseException:
            result.close()
            raise
        return result

    def save(self):
        need(self.guard is not None, 'RUN_OWNER_LOCK_REQUIRED')
        helpers.write_json(self.control / 'state.json', self.state)

    def docker(self, *args):
        need(self.guard is not None, 'RUN_OWNER_LOCK_REQUIRED')
        return self.execute(['docker', '--context', self.state['context'], *args])

    def create_container(self, args):
        need(self.guard is not None, 'RUN_OWNER_LOCK_REQUIRED')
        cidfile = self.control / f'container-{len(self.state["containers"])}.id'
        record = {'cidfile': cidfile.name, 'identity': None}
        self.state['containers'].append(record)
        self.save()  # The cidfile also recovers a lost create response before start.
        self.docker('create', '--cidfile', str(cidfile), '--label', LABEL + '=' + self.state['runId'], *args)
        item = self.inspect_record(record, initial=True)
        record['identity'] = container_identity(item)
        self.save()
        self.docker('start', item['Id'])
        return item['Id']

    def inspect_record(self, record, *, initial=False):
        need(bool(re.fullmatch(r'container-[0-9]+\.id', record['cidfile'])), 'INVALID_CONTAINER_RECEIPT')
        unlinked(self.control / record['cidfile'])
        cid = (self.control / record['cidfile']).read_text().strip()
        need(bool(re.fullmatch('[0-9a-f]{64}', cid)), 'CONTAINER_CREATE_UNKNOWN')
        items = json.loads(self.docker('inspect', cid))
        need(len(items) == 1, 'CONTAINER_IDENTITY_UNVERIFIED')
        item = items[0]
        need(item['Id'] == cid and item['Config']['Labels'].get(LABEL) == self.state['runId'],
             'CONTAINER_IDENTITY_UNVERIFIED')
        need(not item['HostConfig']['AutoRemove'] and item['HostConfig']['RestartPolicy']['Name'] in ('', 'no'),
             'PERSISTENT_CONTAINER_REQUIRED')
        if record['identity'] is not None:
            need(container_identity(item) == record['identity'], 'CONTAINER_CONFIGURATION_CHANGED')
        else:
            need(initial or item['State']['Status'] == 'created', 'CONTAINER_CREATE_UNKNOWN')
        return item

    def spawn(self, name, args, *, env, cwd):
        need(self.guard is not None, 'RUN_OWNER_LOCK_REQUIRED')
        with (self.run / (name + '.log')).open('x', encoding='utf-8') as log:
            p = subprocess.Popen([str(a) for a in args], cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                                 stdout=log, stderr=log, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
        self.children.append(p)
        try:
            record = {'name': name, 'identity': self.process(p.pid)}
            need(record['identity'] is not None, 'NEW_PROCESS_EXITED')
            self.state['processes'].append(record)
            self.save()  # Persist each process immediately, before starting its sibling.
        except BaseException:
            # Popen still owns this newly-created child/handle; a failed birth
            # observation must not orphan it or fall back to a guessed PID.
            p.terminate()
            p.wait(timeout=10)
            raise
        return {'name': name, 'pid': p.pid, 'path': str(args[0] if len(args) == 1 else args[1]),
                'startedAt': time.time(), 'identity': record['identity'], 'runId': self.state['runId']}

    def stop(self):
        if not self.started:
            return None
        need(self.guard is not None, 'RUN_OWNER_LOCK_REQUIRED')
        report = {'schemaVersion': SCHEMA, 'runId': self.state['runId'], 'attemptId': str(uuid.uuid4()),
                  'status': 'FAIL', 'containersStopped': 0, 'processesStopped': 0,
                  'dataAndEvidenceRetained': True, 'removals': 0}
        try:
            need(self.docker('info', '--format', '{{.ID}}') == self.state['daemon'], 'DOCKER_DAEMON_CHANGED')
            containers = [self.inspect_record(record) for record in self.state['containers']]
            # Validate the entire ownership set before signaling anything.
            for record in self.state['processes']:
                expected = record['identity']
                self.process(expected['pid'], expected=expected)
            for record in reversed(self.state['processes']):
                expected = record['identity']
                self.process(expected['pid'], expected=expected, terminate=True)
                deadline = time.monotonic() + 10
                while self.process(expected['pid'], expected=expected) is not None:
                    need(time.monotonic() < deadline, 'PROCESS_STOP_UNCONFIRMED')
                    time.sleep(.05)
                report['processesStopped'] += 1
            for record, item in zip(self.state['containers'], containers):
                self.inspect_record(record)
                if item['State']['Running']:
                    self.docker('stop', '--timeout', '30', item['Id'])
                stopped = self.inspect_record(record)
                need(not stopped['State']['Running'] and stopped['State']['Status'] in ('exited', 'created'),
                     'CONTAINER_STOP_UNCONFIRMED')
                need(stopped['State'].get('ExitCode') == 0 and not stopped['State'].get('OOMKilled'),
                     'CONTAINER_STOP_UNCLEAN')
                report['containersStopped'] += 1
            report['status'] = 'PASS'
        except (CleanupError, OSError, ValueError, KeyError, TypeError) as error:
            report['errorCode'] = 'SHUTDOWN_UNCONFIRMED_RETAIN_RUN'
            report['reasonCode'] = str(error) if isinstance(error, CleanupError) and re.fullmatch('[A-Z_]+', str(error)) else type(error).__name__
            raise CleanupError(report['errorCode']) from None
        finally:
            # The report contains no inspect output, environment, credentials or host paths.
            with (self.run / ('cleanup-' + report['attemptId'] + '.json')).open('x', encoding='utf-8') as f:
                json.dump(report, f, indent=2)
        return report

    def close(self):
        for child in self.children:
            if child.poll() is not None:
                child.wait()
        if self.guard is not None:
            self.guard.__exit__(None, None, None)
            self.guard = None


def finish(scope, action):
    """Failure and interruption both enter stop; never turn acceptance FAIL into PASS."""
    try:
        return action()
    finally:
        try:
            scope.stop()
        finally:
            scope.close()
