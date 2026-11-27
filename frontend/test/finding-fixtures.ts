import type { components } from '../src/shared/api'

type Schema = components['schemas']
export const componentId = '0193a5b0-7777-7000-8000-000000000001'
export const findingProfile = '0193a5b0-4444-7000-8000-000000000003'
export const secretFinding = {
  id: '0193a5b0-e2e2-7000-8000-000000000001',
  finding_kind: 'secret',
  rule_id: 'generic-api-key',
  state: 'open',
  severity: { native: 'ERROR', mapped: 'high', mapping_version: 2 },
  confidence: null,
  location: { path: 'deploy/config.yaml', line_start: 12, line_end: 14 },
  package: null,
  component_id: componentId,
  first_seen_at: '2026-11-08T11:02:00Z',
} satisfies Schema['Finding']
export const acceptedFinding = {
  ...secretFinding,
  id: '0193a5b0-e2e2-7000-8000-000000000002',
  finding_kind: 'security',
  rule_id: 'sql-string-concatenation',
  state: 'accepted_risk',
  confidence: 'high',
  location: { path: 'services/billing/repo.go', line_start: 88, line_end: 88 },
} satisfies Schema['Finding']
export const restrictedFinding = {
  ...acceptedFinding,
  id: '0193a5b0-e2e2-7000-8000-000000000003',
  rule_id: 'url-redirect',
} satisfies Schema['Finding']

export const secretDetail = {
  ...secretFinding,
  evidence: { snippet: 'api_key: [REDACTED]', fingerprint_short: '3f9c2a7b' },
  report: {
    id: '0193a5b0-d4d4-7000-8000-000000000002',
    producer: 'gitleaks',
    producer_version: '8.28.0',
    profile_id: findingProfile,
    mode: 'builtin',
    rules_version: 'r2026.11',
    db_version: null,
    sha: 'a'.repeat(40),
    completeness: 'complete',
  },
  external_triage: [],
  exceptions: [],
  actions: [],
  redactions: [],
} satisfies Schema['FindingDetail']

export const acceptedDetail = {
  ...secretDetail,
  ...acceptedFinding,
  evidence: { rule_description: 'SQL-запрос собран из строк' },
  report: {
    ...secretDetail.report,
    producer: 'semgrep',
    profile_id: null,
    mode: 'import',
    db_version: '2026.11',
    completeness: 'partial',
  },
  exceptions: [
    {
      exception_id: '0193a5b0-5555-7000-8000-000000000001',
      state: 'active',
      expires_at: '2026-12-09T16:20:22Z',
    },
  ],
  external_triage: [
    {
      authority: 'SonarQube',
      state: 'false_positive',
      source_url: 'https://sonar.example/project/issues?id=billing&issues=AYk1',
    },
  ],
  actions: ['create_exception', 'review_exception'],
} satisfies Schema['FindingDetail']
