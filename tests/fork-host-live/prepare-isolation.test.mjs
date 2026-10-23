import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {existsSync} from 'node:fs';
import {mkdir, mkdtemp, readFile, rm, stat, symlink, writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {test} from 'node:test';
import {fileURLToPath} from 'node:url';

const prepare = fileURLToPath(new URL('./prepare.mjs', import.meta.url));

async function scratch(t) {
  const directory = await mkdtemp(path.join(tmpdir(), 'hfraw-prepare-'));
  t.after(async () => {
    assert.equal(path.dirname(path.resolve(directory)), path.resolve(tmpdir()));
    assert.ok(path.basename(directory).startsWith('hfraw-prepare-'));
    await rm(directory, {recursive: true, force: true});
  });
  return directory;
}

function invoke(directory, output) {
  // No Docker executable can be found, even on Windows with a mixed-case Path.
  // The negative tests must fail before external work or credential writes.
  const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => key.toLowerCase() !== 'path'));
  const result = spawnSync(process.execPath, [prepare], {
    cwd: directory,
    env: {...environment, PATH: '', FORK_HOST_LIVE_SYNTHETIC: '1', HFRAW_OUTPUT: output, HFRAW_FORK_WORKTREE: directory},
    encoding: 'utf8', timeout: 10000,
  });
  assert.equal(result.signal, null, 'preparation unexpectedly hung');
  assert.notEqual(result.status, 0, 'the hermetic fixture must not start Docker');
  return result;
}

test('existing credentials are preserved before preparation does any work', async t => {
  const directory = await scratch(t);
  const sentinel = 'retained synthetic credential placeholder';
  for (const name of ['minio-admin.json', 'postgres-dsn']) {
    await writeFile(path.join(directory, name), sentinel, {mode: 0o644});
  }
  invoke(directory, directory);
  for (const name of ['minio-admin.json', 'postgres-dsn']) {
    assert.ok(await readFile(path.join(directory, name), 'utf8') === sentinel, 'existing credential was overwritten');
  }
  assert.equal(existsSync(path.join(directory, 'target.json')), false);
});

test('an existing empty directory is not adopted', async t => {
  const directory = await scratch(t);
  invoke(directory, directory);
  assert.equal(existsSync(path.join(directory, 'target.json')), false);
});

test('a directory link cannot redirect credential writes into a retained directory', async t => {
  const directory = await scratch(t);
  const retained = path.join(directory, 'retained');
  const output = path.join(directory, 'requested-output');
  await mkdir(retained);
  const sentinel = 'retained synthetic credential placeholder';
  await writeFile(path.join(retained, 'minio-admin.json'), sentinel);
  await symlink(retained, output, 'junction');
  invoke(directory, output);
  assert.ok(await readFile(path.join(retained, 'minio-admin.json'), 'utf8') === sentinel, 'linked credential was overwritten');
  assert.equal(existsSync(path.join(retained, 'target.json')), false);
});

test('relative output cannot put private credentials in the current checkout', async t => {
  const directory = await scratch(t);
  invoke(directory, 'relative-trial');
  assert.equal(existsSync(path.join(directory, 'relative-trial')), false);
});

test('a new absolute output is private before the first Docker operation', async t => {
  const directory = await scratch(t);
  const output = path.join(directory, 'new-private-trial');
  const result = invoke(directory, output);
  assert.match(result.stderr, /ENOENT/, 'the isolated run must stop at the unavailable Docker executable');
  assert.equal(JSON.parse(await readFile(path.join(output, 'target.json'), 'utf8')).synthetic, true);
  if (process.platform !== 'win32') {
    assert.equal((await stat(output)).mode & 0o777, 0o700);
    for (const name of ['minio-admin.json', 'postgres-dsn']) {
      assert.equal((await stat(path.join(output, name))).mode & 0o777, 0o600);
    }
  }
});
