import fsd from '@feature-sliced/steiger-plugin'
import { defineConfig } from 'steiger'

export default defineConfig([
  ...fsd.configs.recommended,
  { ignores: ['src/shared/api/generated/**'] },
  {
    // The report screen tasks consume this public API after the shared slice lands.
    files: ['./src/entities/metric/**'],
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
