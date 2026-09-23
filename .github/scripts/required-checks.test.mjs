import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { requiredJobs, assertRequiredChecks } from './required-checks.mjs';

const fullPassing = () => ({
  changes: { result: 'success' },
  ...Object.fromEntries(requiredJobs.map((name) => [name, { result: 'success' }])),
});
const docsPassing = () => ({
  changes: { result: 'success' },
  ...Object.fromEntries(requiredJobs.map((name) => [name, { result: 'skipped' }])),
});

test('all eight groups must succeed for non-doc changes', () => assert.doesNotThrow(() => assertRequiredChecks(fullPassing(), false)));
test('docs-only gate accepts only explicit skips after successful classification', () => assert.doesNotThrow(() => assertRequiredChecks(docsPassing(), true)));

for (const name of requiredJobs) {
  for (const result of ['failure', 'cancelled', 'skipped', 'neutral', '', null]) {
    test(`${name} ${String(result)} blocks full acceptance`, () => {
      const needs = fullPassing(); needs[name].result = result;
      assert.throws(() => assertRequiredChecks(needs, false), /do not satisfy full acceptance/);
    });
  }
  for (const result of ['success', 'failure', 'cancelled', 'neutral', '', null]) {
    test(`${name} ${String(result)} blocks documentation-only skip mode`, () => {
      const needs = docsPassing(); needs[name].result = result;
      assert.throws(() => assertRequiredChecks(needs, true), /do not satisfy documentation-only skip/);
    });
  }
  test(`missing ${name} blocks full acceptance`, () => {
    const needs = fullPassing(); delete needs[name];
    assert.throws(() => assertRequiredChecks(needs, false), /do not satisfy full acceptance/);
  });
  test(`missing ${name} blocks docs-only acceptance`, () => {
    const needs = docsPassing(); delete needs[name];
    assert.throws(() => assertRequiredChecks(needs, true), /do not satisfy documentation-only skip/);
  });
}

for (const result of ['failure', 'cancelled', 'skipped', 'neutral', '', null]) {
  test(`change classifier ${String(result)} blocks full acceptance`, () => {
    const needs = fullPassing(); needs.changes.result = result;
    assert.throws(() => assertRequiredChecks(needs, false), /classification did not succeed/);
  });
  test(`change classifier ${String(result)} blocks docs-only acceptance`, () => {
    const needs = docsPassing(); needs.changes.result = result;
    assert.throws(() => assertRequiredChecks(needs, true), /classification did not succeed/);
  });
}

for (const needs of [null, undefined, [], {}, 'success']) {
  test(`invalid needs ${JSON.stringify(needs)} blocks required`, () => assert.throws(() => assertRequiredChecks(needs)));
}
test('inherited success cannot satisfy missing jobs', () => assert.throws(() => assertRequiredChecks(Object.create(fullPassing()), false)));
test('unknown failed dependency cannot be ignored', () => assert.throws(() => assertRequiredChecks({ ...fullPassing(), future: { result: 'failure' } }, false)));
test('command exits nonzero on malformed environment', () => {
  const result = spawnSync(process.execPath, [fileURLToPath(new URL('./required-checks.mjs', import.meta.url))], { env: { ...process.env, CI_NEEDS: '{bad json', CI_DOCS_ONLY: 'false' } });
  assert.equal(result.status, 1);
});
test('command accepts explicit docs-only skipped dependencies', () => {
  const result = spawnSync(process.execPath, [fileURLToPath(new URL('./required-checks.mjs', import.meta.url))], { env: { ...process.env, CI_NEEDS: JSON.stringify(docsPassing()), CI_DOCS_ONLY: 'true' }, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
});
