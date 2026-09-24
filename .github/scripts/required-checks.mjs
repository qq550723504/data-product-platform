import { pathToFileURL } from 'node:url';

export const requiredJobs = Object.freeze([
  'go-platform', 'web-console', 'hop-smoke', 'splink-smoke',
  'label-studio-smoke', 'browser-contracts', 'live-core', 'demo',
]);

export function assertRequiredChecks(needs, docsOnly = false) {
  if (!needs || typeof needs !== 'object' || Array.isArray(needs)) throw new Error('CI needs must be an object');
  if (needs.changes?.result !== 'success') throw new Error('Change classification did not succeed');

  const expectedResult = docsOnly ? 'skipped' : 'success';
  const problems = requiredJobs.filter((name) => !Object.hasOwn(needs, name) || needs[name]?.result !== expectedResult);

  for (const [name, job] of Object.entries(needs)) {
    if (name === 'changes' || requiredJobs.includes(name)) continue;
    if (job?.result !== 'success') problems.push(name);
  }

  if (problems.length) {
    const mode = docsOnly ? 'documentation-only skip' : 'full acceptance';
    throw new Error(`Required jobs do not satisfy ${mode}: ${problems.join(', ')}`);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const docsOnly = process.env.CI_DOCS_ONLY === 'true';
    assertRequiredChecks(JSON.parse(process.env.CI_NEEDS ?? 'null'), docsOnly);
    console.log(docsOnly
      ? 'Documentation-only change classified successfully; all eight heavy job groups were skipped.'
      : 'All eight required job groups completed successfully.');
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Invalid gate input');
    process.exitCode = 1;
  }
}
