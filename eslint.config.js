import js from '@eslint/js';
import globals from 'globals';

export default [
  {
    ignores: [
      'node_modules/**',
      '.test-artifacts/**',
      '.superpowers/**',
      'dist/**',
      'bin/**',
      'tests/fixtures/**',
    ],
  },
  js.configs.recommended,
  {
    files: ['**/*.{js,cjs,mjs}'],
    rules: { 'no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }] },
  },
  {
    files: ['extension/**/*.js'],
    languageOptions: { globals: { ...globals.browser, chrome: 'readonly' } },
  },
  {
    files: ['scripts/**/*.{js,cjs,mjs}', 'eslint.config.js'],
    languageOptions: { globals: globals.node },
  },
  {
    files: ['tests/**/*.{js,cjs,mjs}', 'scripts/store-screenshots.cjs'],
    languageOptions: { globals: { ...globals.node, ...globals.browser, chrome: 'readonly' } },
  },
  {
    files: ['tests/extractor.test.cjs'],
    languageOptions: {
      globals: {
        PageVaultExtractor: 'readonly',
        activeObservers: 'readonly',
        submissions: 'writable',
      },
    },
  },
  {
    files: ['tests/onboarding.test.cjs'],
    languageOptions: { globals: { releaseChecks: 'readonly' } },
  },
  {
    files: ['tests/ui.test.cjs'],
    languageOptions: {
      globals: { fixture: 'readonly', releaseList: 'readonly', domainOffsets: 'readonly' },
    },
  },
];
