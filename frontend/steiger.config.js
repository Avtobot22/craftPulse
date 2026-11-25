import fsd from '@feature-sliced/steiger-plugin'
import { defineConfig } from 'steiger'

export default defineConfig([
  ...fsd.configs.recommended,
  { ignores: ['src/shared/api/generated/**'] },
  {
    // Report, task analysis and technical screens consume these public APIs as they land.
    files: [
      './src/entities/metric/**',
      './src/entities/activity/**',
      './src/entities/merge-request/**',
      './src/entities/check-report/**',
      './src/entities/finding/**',
      './src/entities/result-access/**',
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
