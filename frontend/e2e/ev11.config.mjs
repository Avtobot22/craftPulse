import { defineConfig } from '@playwright/test'
import ac01 from './playwright.config.mjs'

export default defineConfig({
  ...ac01,
  testMatch: 'ev11.stand.mjs',
  reporter: [['./reporter.mjs', { check: 'EV-11', count: 4 }]],
})
