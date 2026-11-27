import type { components } from '@/shared/api'
import { currentUser } from './fixtures'

type Schema = components['schemas']

export const service = {
  id: '0193a5b0-6666-7000-8000-000000000001',
  name: 'Billing',
  status: 'active',
} satisfies Schema['Service']
export const root = {
  id: '0193a5b0-e4e4-7000-8000-000000000001',
  area_id: '0193a5b0-8888-7000-8000-000000000001',
  path_prefix: 'docs',
  profile_id: 'git-markdown-openapi',
  owner_person_id: null,
  audience: 'team',
} satisfies Schema['DocumentationRoot']
export const owner = {
  ...currentUser,
  areas: { ...currentUser.areas, services: [service] },
  actions: ['session.read', 'docs.read', 'docs.manage', 'findings.read'],
  roles: [
    {
      role: 'service_owner',
      service_id: service.id,
      project_id: null,
      team_id: null,
      assigned_at: '2026-11-08T11:02:00Z',
    },
  ],
} satisfies Schema['CurrentUser']

const sha = '7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e'
const timestamp = '2026-11-08T11:02:00Z'

export const documentPage = {
  documentation_state: 'ASSESSED',
  data: [
    {
      id: '0193a5b0-e5e5-7000-8000-000000000001',
      title: 'Руководство Billing',
      doc_format: 'markdown',
      path: 'docs/README.md',
      version_ref: sha,
      last_checked_at: timestamp,
      open_violations: 1,
      source_url:
        'https://gitlab.example/core/-/blob/' + sha + '/docs/README.md',
    },
  ],
  next_cursor: null,
  access_limited: false,
  access_notes: [],
} satisfies Schema['DocumentPage']

export const emptyCorpus = {
  ...documentPage,
  documentation_state: 'UNKNOWN',
  data: [],
} satisfies Schema['DocumentPage']

export const anchorFinding = {
  id: '0193a5b0-e2e2-7000-8000-000000000011',
  finding_kind: 'docs',
  rule_id: 'missing-anchor',
  state: 'open',
  severity: { native: 'error', mapped: 'high', mapping_version: 1 },
  confidence: null,
  location: { path: 'docs/README.md', line_start: 12, line_end: 12 },
  package: null,
  component_id: null,
  first_seen_at: timestamp,
} satisfies Schema['Finding']

export const anchorDetail = {
  ...anchorFinding,
  area_id: root.area_id,
  source_url: documentPage.data[0]?.source_url ?? '',
  evidence: { target_path: 'docs/guide.md', anchor: 'install' },
  report: {
    id: '0193a5b0-d4d4-7000-8000-000000000011',
    producer: 'goldmark',
    producer_version: '1.8.2',
    profile_id: root.profile_id,
    mode: 'builtin',
    sha,
    rules_version: null,
    db_version: null,
    completeness: 'complete',
  },
  external_triage: [],
  exceptions: [],
  actions: [],
  redactions: [],
} satisfies Schema['FindingDetail']

export const specificationFinding = {
  ...anchorFinding,
  id: '0193a5b0-e2e2-7000-8000-000000000012',
  finding_kind: 'openapi',
  rule_id: 'oas3-schema',
  location: { path: 'docs/openapi.yaml', line_start: 8, line_end: 8 },
} satisfies Schema['Finding']

export const specificationDetail = {
  ...anchorDetail,
  ...specificationFinding,
  source_url:
    'https://gitlab.example/core/-/blob/' + sha + '/docs/openapi.yaml',
  evidence: { rule_description: 'Невалидная схема ответа' },
  report: {
    ...anchorDetail.report,
    producer: 'spectral',
    producer_version: '6.15.0',
  },
} satisfies Schema['FindingDetail']

export const acceptedCheck = {
  id: '0193a5b0-a0a0-7000-8000-000000000011',
  kind: 'documentation_check',
  status: 'queued',
  created_at: timestamp,
  updated_at: timestamp,
  result_uri: null,
  error: null,
  retry_after_seconds: 1,
} satisfies Schema['BackgroundOperation']
