import { fileURLToPath } from 'node:url';
import { ESLint } from 'eslint';
import { describe, expect, it } from 'vitest';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const eslint = new ESLint({ cwd: root, overrideConfigFile: `${root}eslint.config.js` });

// Only the ban rule is under test; unrelated rules (e.g. floating promises) are out of scope here.
async function errorsFor(code: string, filePath = 'packages/vitals-web/src/x.ts'): Promise<string[]> {
  const [result] = await eslint.lintText(code, { filePath: `${root}${filePath}` });
  return (result?.messages ?? [])
    .filter((m) => m.ruleId === 'no-restricted-globals' || m.fatal === true)
    .map((m) => m.ruleId ?? 'parse-error');
}

describe('network and storage ban (spec §3)', () => {
  it.each(["fetch('/x')", 'localStorage.setItem("a","b")', 'new WebSocket("ws://x")', 'indexedDB.open("x")'])(
    'rejects %s',
    async (code) => {
      expect(await errorsFor(code)).toContain('no-restricted-globals');
    },
  );

  it('allows the worker assets loader', async () => {
    expect(await errorsFor("fetch('/x')", 'packages/vitals-web/src/worker/assets.ts')).toEqual([]);
  }, 30_000);
});
