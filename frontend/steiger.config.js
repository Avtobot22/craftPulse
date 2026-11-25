import fsd from '@feature-sliced/steiger-plugin'
import { defineConfig } from 'steiger'

export default defineConfig([
  ...fsd.configs.recommended,
  { ignores: ['src/shared/api/generated/**'] },
  {
    // The report and task analysis screens will consume these public APIs as they land.
    files: [
      './src/entities/metric/**',
      './src/entities/activity/**',
      './src/entities/merge-request/**',
    ],
    rules: { 'fsd/insignificant-slice': 'off' },
  },
  {
    files: ['src/**'],
    rules: {
      'fsd/forbidden-imports': 'error',
      'fsd/no-public-api-sidestep': 'error',
    },
  },
])
