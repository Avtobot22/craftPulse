import { readdirSync } from 'node:fs'
import { defineConfig } from '@playwright/test'
import { settings, profiles } from './settings.mjs'

const run = settings()
const use = profiles[run.profile]
if (readdirSync(run.output).length)
  throw new Error('Use a fresh AC01_EVIDENCE_DIR')

export default defineConfig({
  testDir: '.',
  testMatch: 'ac01.stand.mjs',
  // Each profile starts from the pre-bootstrap snapshot and mutates that installation.
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: true,
  timeout: 120_000,
  expect: { timeout: 15_000 },
  reporter: [['./reporter.mjs']],
  outputDir: run.output + '/playwright',
  projects: [{ name: run.profile, use }],
  use: {
    baseURL: run.site,
    actionTimeout: 15_000,
    navigationTimeout: 20_000,
    trace: 'off',
    screenshot: 'off',
    video: 'off',
  },
})
