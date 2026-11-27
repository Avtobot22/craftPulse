import fsd from '@feature-sliced/steiger-plugin'
import { defineConfig } from 'steiger'

export default defineConfig([
  ...fsd.configs.recommended,
  {
    // Candidate decisions are a feature in the UI-25 requirements.
    files: ['./src/features/decide-link/**'],
    rules: { 'fsd/insignificant-slice': 'off' },
  },
  { ignores: ['src/shared/api/generated/**'] },
  {
    // Report, task analysis, technical and administration screens consume these public APIs as they land.
    files: [
      './src/entities/metric/**',
      './src/entities/activity/**',
      './src/entities/merge-request/**',
      './src/entities/check-report/**',
      './src/entities/finding/**',
      './src/entities/result-access/**',
      './src/entities/connection/**',
      './src/entities/sync/**',
      './src/entities/receipt/**',
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
