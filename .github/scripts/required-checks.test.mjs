import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { requiredJobs, assertRequiredChecks } from './required-checks.mjs';
const passing = () => Object.fromEntries(requiredJobs.map((name) => [name, { result: 'success' }]));
test('all seven groups must succeed', () => assert.doesNotThrow(() => assertRequiredChecks(passing())));
for (const name of requiredJobs) {
  for (const result of ['failure', 'cancelled', 'skipped', 'neutral', '', null]) {
    test(`${name} ${String(result)} blocks required`, () => {
      const needs = passing(); needs[name].result = result;
      assert.throws(() => assertRequiredChecks(needs), /not successful/);
    });
  }
  test(`missing ${name} blocks required`, () => {
    const needs = passing(); delete needs[name];
    assert.throws(() => assertRequiredChecks(needs), /not successful/);
  });
}
for (const needs of [null, undefined, [], {}, 'success']) {
  test(`invalid needs ${JSON.stringify(needs)} blocks required`, () => assert.throws(() => assertRequiredChecks(needs)));
}
test('inherited success cannot satisfy missing jobs', () => assert.throws(() => assertRequiredChecks(Object.create(passing()))));
test('unknown failed dependency cannot be ignored', () => assert.throws(() => assertRequiredChecks({ ...passing(), future: { result: 'failure' } })));
test('command exits nonzero on malformed environment', () => {
  const result = spawnSync(process.execPath, [fileURLToPath(new URL('./required-checks.mjs', import.meta.url))], { env: { ...process.env, CI_NEEDS: '{bad json' } });
  assert.equal(result.status, 1);
});
