import eslint from '@eslint/js';
import tseslint from 'typescript-eslint';

const BANNED = ['fetch', 'XMLHttpRequest', 'WebSocket', 'EventSource', 'localStorage', 'sessionStorage', 'indexedDB'];

export default tseslint.config(
  { ignores: ['**/dist/**', '**/node_modules/**', '**/coverage/**', 'engine/**'] },
  {
    files: ['packages/**/src/**/*.ts', 'apps/**/src/**/*.ts', 'tools/**/src/**/*.ts'],
    extends: [eslint.configs.recommended, ...tseslint.configs.strictTypeChecked],
    languageOptions: {
      parserOptions: {
        projectService: {
          defaultProject: 'packages/vitals-web/tsconfig.json',
          // Virtual files used by lint-rules.test.ts do not exist on disk.
          allowDefaultProject: ['packages/vitals-web/src/x.ts'],
        },
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      'no-restricted-globals': [
        'error',
        ...BANNED.map((name) => ({ name, message: 'Network and storage are banned (spec §3)' })),
      ],
    },
  },
  {
    // Outside the typed projects: untyped recommended rules only.
    files: ['apps/demo/e2e/**/*.ts', 'apps/demo/*.mjs', 'tools/**/*.mjs'],
    extends: [eslint.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: { globals: { process: 'readonly', console: 'readonly', URL: 'readonly' } },
  },
  {
    files: ['packages/vitals-web/src/worker/assets.ts'],
    rules: { 'no-restricted-globals': 'off' },
  },
);
