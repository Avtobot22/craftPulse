import type { components } from '../src/shared/api'

export const currentUser = {
  user: {
    id: '0193a5b0-1111-7000-8000-000000000001',
    display_name: 'Анна Смирнова',
    status: 'active',
    person_id: '0193a5b0-2222-7000-8000-000000000001',
  },
  organization: {
    id: '0193a5b0-0000-7000-8000-000000000001',
    name: 'Пример',
    slug: 'example',
    status: 'active',
  },
  roles: [
    {
      role: 'dev_lead',
      team_id: '0193a5b0-3333-7000-8000-000000000001',
      project_id: null,
      service_id: null,
      assigned_at: '2026-10-01T09:00:00Z',
    },
  ],
  areas: {
    teams: [
      {
        id: '0193a5b0-3333-7000-8000-000000000001',
        name: 'Платформа',
        status: 'active',
      },
    ],
    projects: [
      {
        id: '0193a5b0-4444-7000-8000-000000000001',
        name: 'Core',
        status: 'active',
      },
    ],
    services: [],
  },
  actions: ['session.read', 'report.read', 'tasks.read', 'export.create'],
  session: {
    expires_at: '2026-11-08T21:00:00Z',
    idle_expires_at: '2026-11-08T10:00:00Z',
  },
  review_participation: {
    participates: false,
  },
} satisfies components['schemas']['CurrentUser']

export const teams = {
  data: [
    {
      id: '0193a5b0-3333-7000-8000-000000000001',
      name: 'Платформа',
      status: 'active',
    },
  ],
  next_cursor: null,
} satisfies components['schemas']['TeamPage']

export const projects = {
  data: [
    {
      id: '0193a5b0-4444-7000-8000-000000000001',
      name: 'Core',
      status: 'active',
    },
  ],
  next_cursor: null,
  access_limited: false,
  access_notes: [],
} satisfies components['schemas']['ProjectPage']

export const services = {
  data: [
    {
      id: '0193a5b0-6666-7000-8000-000000000001',
      name: 'billing',
      status: 'active',
    },
  ],
  next_cursor: null,
} satisfies components['schemas']['ServicePage']

export const structure = {
  version: 7,
  teams: [
    {
      id: '0193a5b0-3333-7000-8000-000000000001',
      name: 'Платформа',
      status: 'active',
    },
  ],
  projects: [
    {
      id: '0193a5b0-4444-7000-8000-000000000001',
      name: 'Core',
      status: 'active',
    },
  ],
  services: [
    {
      id: '0193a5b0-6666-7000-8000-000000000001',
      name: 'billing',
      status: 'active',
    },
  ],
  components: [
    {
      id: '0193a5b0-7777-7000-8000-000000000001',
      service_id: '0193a5b0-6666-7000-8000-000000000001',
      repository_area_id: '0193a5b0-8888-7000-8000-000000000001',
      path_prefix: 'services/billing',
      status: 'proposed',
      origin: 'manifest',
    },
  ],
  team_projects: [
    {
      team_id: '0193a5b0-3333-7000-8000-000000000001',
      project_id: '0193a5b0-4444-7000-8000-000000000001',
      valid_from: '2026-10-01T00:00:00Z',
      valid_to: null,
    },
  ],
  project_areas: [
    {
      project_id: '0193a5b0-4444-7000-8000-000000000001',
      area_id: '0193a5b0-8888-7000-8000-000000000001',
      valid_from: '2026-10-01T00:00:00Z',
      valid_to: null,
    },
  ],
  team_members: [
    {
      team_id: '0193a5b0-3333-7000-8000-000000000001',
      person_id: '0193a5b0-2222-7000-8000-000000000001',
      valid_from: '2026-10-01T00:00:00Z',
      valid_to: null,
    },
  ],
} satisfies components['schemas']['OrganizationStructure']

export const structureInput = {
  teams: [
    {
      id: '0193a5b0-3333-7000-8000-000000000001',
      name: 'Платформа',
      status: 'active',
    },
  ],
  projects: [
    {
      id: '0193a5b0-4444-7000-8000-000000000001',
      name: 'Core',
      status: 'active',
    },
  ],
  services: [
    {
      id: '0193a5b0-6666-7000-8000-000000000001',
      name: 'billing',
      status: 'active',
    },
  ],
  components: [
    {
      id: '0193a5b0-7777-7000-8000-000000000001',
      service_id: '0193a5b0-6666-7000-8000-000000000001',
      repository_area_id: '0193a5b0-8888-7000-8000-000000000001',
      path_prefix: 'services/billing',
      status: 'confirmed',
    },
  ],
  team_projects: [
    {
      team_id: '0193a5b0-3333-7000-8000-000000000001',
      project_id: '0193a5b0-4444-7000-8000-000000000001',
      valid_from: null,
      valid_to: null,
    },
  ],
  project_areas: [
    {
      project_id: '0193a5b0-4444-7000-8000-000000000001',
      area_id: '0193a5b0-8888-7000-8000-000000000001',
      valid_from: null,
      valid_to: null,
    },
  ],
  team_members: [
    {
      team_id: '0193a5b0-3333-7000-8000-000000000001',
      person_id: '0193a5b0-2222-7000-8000-000000000001',
      valid_from: null,
      valid_to: null,
    },
  ],
} satisfies components['schemas']['OrganizationStructureInput']

export const correlationId = '0193a5b0-9999-7000-8000-000000000001'
export function problem(
  code: string,
  status: number,
): components['schemas']['Problem'] {
  return {
    type: 'urn:craftpulse:problem:' + code,
    title: 'Ошибка запроса',
    status,
    code,
    correlation_id: correlationId,
    detail: 'SQL private-internal.example secret',
  }
}
export function jsonResponse(
  data: unknown,
  status = 200,
  headers: HeadersInit = {},
): Response {
  const resultHeaders = new Headers(headers)
  if (!resultHeaders.has('Content-Type'))
    resultHeaders.set('Content-Type', 'application/json')
  return new Response(JSON.stringify(data), { status, headers: resultHeaders })
}
export function problemResponse(code: string, status: number): Response {
  return jsonResponse(problem(code, status), status, {
    'Content-Type': 'application/problem+json',
  })
}

// Example from listProjectTimeline in OpenAPI.
export const lateTimelineEvent = {
  id: '0193a5b0-e1e1-7000-8000-000000000001',
  object: {
    kind: 'task',
    id: '0193a5b0-f000-7000-8000-000000000002',
    display_key: 'CORE-15',
    title: 'Перенести отчёт',
    source_url: 'https://example.atlassian.net/browse/CORE-15',
  },
  event_kind: 'field_change',
  field: 'status',
  before_value: 'Open',
  after_value: 'In Progress',
  actor: {
    account_id: '0193a5b0-9999-7000-8000-000000000001',
    kind: 'human',
    display_name: 'Анна Смирнова',
  },
  event_time: '2026-10-02T09:00:00Z',
  event_offset_seconds: 10800,
  observed_at: '2026-10-06T00:00:00Z',
  observed_via: 'backfill',
  late: true,
} satisfies components['schemas']['TimelineEvent']
