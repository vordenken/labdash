import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { createRequire } from 'node:module';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(import.meta.url);
const { newerStableTag } = require('../scripts/release-policy.cjs');

function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'labdash-release-test-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  writeFileSync(join(dir, 'semver.txt'), '1.2.3\n---\nRelease notes\n');
  writeFileSync(join(dir, 'git'), `#!/bin/bash
case "$1" in
  merge-base) echo previous ;;
  cat-file)
    if [[ "\${3:-}" == *:semver.txt && "\${NO_PREVIOUS:-}" == true ]]; then exit 1; fi ;;
  show) echo "\${PREVIOUS_VERSION:-1.2.3}" ;;
  diff) exit "\${DIFF_STATUS:-1}" ;;
  tag) printf '%s\\n' "\${TAGS:-}" ;;
  rev-list) if [[ "\${4:-}" == "\${MATCH_TAG:-}" ]]; then echo head; else echo other; fi ;;
  show-ref) [[ "\${4:-}" == "refs/tags/\${EXISTING_TAG:-}" ]] ;;
  *) exit 2 ;;
esac
`, { mode: 0o755 });
  const run = (command, overrides = {}) => {
    const output = join(dir, 'output');
    writeFileSync(output, '');
    const env = {
      ...process.env, PATH: `${dir}:${process.env.PATH}`,
      EVENT_NAME: 'push', REF_NAME: 'main', HEAD_SHA: 'head', BEFORE: 'previous',
      GITHUB_OUTPUT: output, ...overrides,
    };
    execFileSync('bash', [join(root, 'scripts/release.sh'), command], { cwd: dir, env, stdio: 'pipe' });
    return Object.fromEntries(readFileSync(output, 'utf8').trim().split('\n').map((line) => line.split('=')));
  };
  run.dir = dir;
  return run;
}

test('release trigger requires a new main version or relevant feature changes', (t) => {
  const run = fixture(t);
  assert.equal(run('check')['should-build'], 'false');
  assert.equal(run('check', { PREVIOUS_VERSION: '1.2.2' })['should-build'], 'true');
  assert.equal(run('check', { NO_PREVIOUS: 'true' })['should-build'], 'true');
  assert.equal(run('check', { EVENT_NAME: 'workflow_dispatch' })['should-build'], 'true');
  assert.equal(run('check', { REF_NAME: 'bugfix/example' })['should-build'], 'false');
  assert.equal(run('check', { REF_NAME: 'feature/example', DIFF_STATUS: '0' })['should-build'], 'false');
  assert.equal(run('check', { REF_NAME: 'feature/example' })['should-build'], 'true');
  assert.equal(run('check', {
    REF_NAME: 'feature/example', BEFORE: '0000000000000000000000000000000000000000', DIFF_STATUS: '0',
  })['should-build'], 'false');
  assert.throws(() => run('check', { REF_NAME: 'feature/example', DIFF_STATUS: '128' }), /Cannot compare/);
});

test('release metadata creates stable and retryable numbered beta versions', (t) => {
  const run = fixture(t);
  assert.deepEqual(run('prepare'), {
    tag: 'v1.2.3', version: '1.2.3', 'image-version': '1.2.3',
    major: '1', 'major-minor': '1.2', prerelease: 'false',
  });
  const beta = { REF_NAME: 'feature/example', TAGS: 'v1.2.3-beta1\nv1.2.3-beta2' };
  assert.equal(run('prepare', beta).tag, 'v1.2.3-beta3');
  const retry = run('prepare', { ...beta, MATCH_TAG: 'v1.2.3-beta1', EXISTING_TAG: 'v1.2.3-beta1' });
  assert.equal(retry.tag, 'v1.2.3-beta1');
  assert.equal(retry['image-version'], '1.2.3-beta1');
  assert.equal(retry.prerelease, 'true');
  assert.throws(() => run('prepare', { EXISTING_TAG: 'v1.2.3' }), /refusing to move/);
});

test('release metadata rejects invalid versions and missing release notes', (t) => {
  const run = fixture(t);
  writeFileSync(join(run.dir, 'semver.txt'), '01.2.3\n---\nNotes\n');
  assert.throws(() => run('prepare'), /Invalid version/);
  writeFileSync(join(run.dir, 'semver.txt'), '1.2.3\n---\n  \n');
  assert.throws(() => run('prepare'), /no current release notes/);
  writeFileSync(join(run.dir, 'semver.txt'), '1.2.3\nmissing separator\nNotes\n');
  assert.throws(() => run('prepare'), /must have ---/);
});

test('a newer stable tag prevents rolling back latest', () => {
  assert.equal(newerStableTag('1.2.3', ['v1.2.2', 'v1.2.3', 'v2.0.0-beta1']), null);
  assert.equal(newerStableTag('1.2.3', ['v1.2.10']), 'v1.2.10');
  assert.equal(newerStableTag('1.9.9', ['v2.0.0']), 'v2.0.0');
  assert.equal(newerStableTag('2.0.0', ['v1.999.999']), null);
  assert.equal(newerStableTag('1.2.3', ['v01.2.4', 'other']), null);
});
