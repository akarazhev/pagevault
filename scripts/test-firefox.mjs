import { spawnSync } from 'node:child_process';

const result = spawnSync(
  process.execPath,
  [
    '--test',
    '--test-concurrency=1',
    'tests/extractor.test.cjs',
    'tests/queue.test.cjs',
    'tests/ui.test.cjs',
    'tests/onboarding.test.cjs',
    'tests/firefox-integration.test.cjs',
  ],
  { stdio: 'inherit', env: { ...process.env, PAGEVAULT_TEST_BROWSER: 'firefox' } },
);
if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
