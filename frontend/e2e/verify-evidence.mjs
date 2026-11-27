import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { accounts, password, profiles, settings } from './settings.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const { output, profile, sha, browserVersion } = settings()
const cookie = process.env.AC01_OBSERVED_SESSION
if (!cookie)
  throw new Error('Supply AC01_OBSERVED_SESSION from the manual session check')
const secrets = [...Object.keys(accounts).map(password), cookie]
const patterns = secrets.flatMap((value) => [
  Buffer.from(value),
  Buffer.from(JSON.stringify(value).slice(1, -1)),
  Buffer.from(encodeURIComponent(value)),
])

function inspect(path) {
  if (!lstatSync(path).isFile()) return
  const data = readFileSync(path)
  if (patterns.some((pattern) => data.includes(pattern))) {
    throw new Error('A protected value matched a checked file')
  }
}

const worktreeFiles = execFileSync(
  'git',
  ['ls-files', '-z', '--cached', '--others', '--exclude-standard'],
  {
    cwd: repo,
    encoding: 'utf8',
  },
)
  .split('\0')
  .filter(Boolean)
for (const file of new Set(worktreeFiles)) inspect(join(repo, file))

for (const entry of readdirSync(output, {
  recursive: true,
  withFileTypes: true,
})) {
  if (entry.isSymbolicLink()) throw new Error('Unexpected evidence symlink')
  inspect(join(entry.parentPath, entry.name))
}

const runData = readFileSync(join(output, 'run.json'))
const report = JSON.parse(runData)
const expected = profiles[profile]
if (
  report.check !== 'AC-01' ||
  report.commit_sha !== sha ||
  report.browser !== expected.browserName ||
  (report.browser_version !== browserVersion &&
    !(report.browser_version === null && report.outcome === 'failed')) ||
  report.viewport?.width !== expected.viewport.width ||
  report.viewport?.height !== expected.viewport.height ||
  !Array.isArray(report.scenarios) ||
  report.scenarios.map(({ id }) => id).join(',') !== '01,02,03,04,05,06,07'
) {
  throw new Error('Evidence does not match the requested AC-01 run')
}
for (const { id, trace } of report.scenarios) {
  if (!trace) continue
  if (trace.file !== `trace-${id}.json`)
    throw new Error('Unexpected trace filename')
  const data = readFileSync(join(output, trace.file))
  if (createHash('sha256').update(data).digest('hex') !== trace.sha256) {
    throw new Error('Trace SHA-256 mismatch')
  }
}
process.stdout.write(
  `Evidence and worktree secret scan passed. run.json SHA-256 ${createHash('sha256').update(runData).digest('hex')}\n`,
)
