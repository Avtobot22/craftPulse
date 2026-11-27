import { createHash } from 'node:crypto'
import { writeFileSync } from 'node:fs'
import { platform, release, version } from 'node:os'
import { join } from 'node:path'
import { accounts, password, profiles, settings } from './settings.mjs'

export default class EvidenceReporter {
  run = settings()
  records = new Map()
  failedSteps = new Map()
  runtimeError = false

  onBegin(_config, suite) {
    for (const test of suite.allTests()) {
      const id = /^\[(0[1-7])\] /.exec(test.title)?.[1]
      if (!id || this.records.has(id))
        throw new Error('Unexpected AC-01 test ID')
      this.records.set(id, { id, status: 'not-run' })
    }
  }

  onStepEnd(test, _result, step) {
    if (step.category !== 'test.step' || !step.error) return
    const id = /^\[(0[1-7])\] /.exec(test.title)?.[1]
    // Titles are authored static literals. Never serialize Playwright errors, API
    // calls, hook output, page content, request data or attachments from failures.
    if (!id || !/^[a-z][a-z0-9 -]{1,79}$/.test(step.title)) return
    const rows = this.failedSteps.get(id) ?? []
    rows.push({
      step: step.title,
      started_at: step.startTime.toISOString(),
      duration_ms: step.duration,
    })
    this.failedSteps.set(id, rows)
  }

  onTestEnd(test, result) {
    const id = /^\[(0[1-7])\] /.exec(test.title)?.[1]
    const record = this.records.get(id)
    if (!record) return
    record.status = result.status
    const attachment = result.attachments.find(
      ({ name }) => name === 'verified-browser',
    )
    if (attachment?.body?.toString('utf8') === this.run.browserVersion) {
      this.browserVersion = this.run.browserVersion
    }
  }

  onError() {
    this.runtimeError = true
  }

  // Keep Playwright from adding a fallback stdout reporter with raw errors.
  printsToStdio() {
    return true
  }

  // Diagnostic errors and browser console output can contain OAuth material.
  onStdOut() {}
  onStdErr() {}

  write(name, value) {
    const data = JSON.stringify(value, null, 2) + '\n'
    for (const account of Object.keys(accounts)) {
      const secret = password(account)
      const escaped = JSON.stringify(secret)
      if (
        data.includes(escaped) ||
        (secret.length >= 8 && data.includes(escaped.slice(1, -1)))
      ) {
        throw new Error('A protected value matched the evidence')
      }
    }
    writeFileSync(join(this.run.output, name), data, {
      flag: 'wx',
      mode: 0o600,
    })
    return {
      file: name,
      sha256: createHash('sha256').update(data).digest('hex'),
    }
  }

  onEnd(result) {
    try {
      return this.finish(result)
    } catch {
      process.stderr.write('Could not write AC-01 evidence\n')
      return { status: 'failed' }
    }
  }

  finish(result) {
    const expected = Array.from({ length: 7 }, (_, index) => '0' + (index + 1))
    const scenarios = expected.map(
      (id) => this.records.get(id) ?? { id, status: 'not-run' },
    )
    for (const scenario of scenarios) {
      const steps = this.failedSteps.get(scenario.id)
      if (steps?.length)
        scenario.trace = this.write('trace-' + scenario.id + '.json', steps)
    }
    const passed =
      result.status === 'passed' &&
      !this.runtimeError &&
      scenarios.every(({ status }) => status === 'passed')
    const profile = profiles[this.run.profile]
    const record = this.write('run.json', {
      check: 'AC-01',
      outcome: passed ? 'passed' : 'failed',
      started_at: result.startTime.toISOString(),
      duration_ms: result.duration,
      commit_sha: this.run.sha,
      playwright_version: '1.65.0',
      browser: profile.browserName,
      browser_version: this.browserVersion ?? null,
      os: { platform: platform(), release: release(), version: version() },
      viewport: profile.viewport,
      scenarios,
    })
    process.stdout.write(
      `AC-01 ${this.run.profile} ${passed ? 'passed' : 'failed'} run.json SHA-256 ${record.sha256}\n`,
    )
    if (!passed) return { status: 'failed' }
  }
}
