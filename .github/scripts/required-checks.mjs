import { pathToFileURL } from 'node:url';

export const requiredJobs = Object.freeze([
  'go-platform', 'web-console', 'hop-smoke', 'splink-smoke',
  'browser-contracts', 'live-core', 'demo',
]);

export function assertRequiredChecks(needs) {
  if (!needs || typeof needs !== 'object' || Array.isArray(needs)) throw new Error('CI needs must be an object');
  const problems = requiredJobs.filter((name) => !Object.hasOwn(needs, name) || needs[name]?.result !== 'success');
  for (const [name, job] of Object.entries(needs)) {
    if (!requiredJobs.includes(name) && job?.result !== 'success') problems.push(name);
  }
  if (problems.length) throw new Error(`Required jobs not successful: ${problems.join(', ')}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    assertRequiredChecks(JSON.parse(process.env.CI_NEEDS ?? 'null'));
    console.log('All seven required job groups completed successfully.');
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Invalid gate input');
    process.exitCode = 1;
  }
}
