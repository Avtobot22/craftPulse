import type { components } from '../src/shared/api'
import { lateTimelineEvent } from './fixtures'

type Schema = components['schemas']

export const taskId = '0193a5b0-f000-7000-8000-000000000002'
export const taskTitle = 'CORE-15 · Перенести отчёт'
export const readyAt = '2026-11-06T12:00:00Z'
export const taskReference = {
  kind: 'task',
  id: taskId,
  display_key: 'CORE-15',
  title: 'Перенести отчёт',
  source_url: 'https://jira.example/browse/CORE-15',
} satisfies Schema['ObjectRef']

export const mrReference = {
  kind: 'merge_request',
  id: '0193a5b0-b0b0-7000-8000-000000000001',
  display_key: '!42',
  title: 'Перенос отчёта',
  source_url: 'https://gitlab.example/core/-/merge_requests/42',
} satisfies Schema['ObjectRef']

export const pipelineReference = {
  kind: 'pipeline',
  id: '0193a5b0-d0d0-7000-8000-000000000001',
  display_key: '#8841',
  title: null,
  source_url: 'https://gitlab.example/core/-/pipelines/8841',
} satisfies Schema['ObjectRef']

export const taskDetail: Schema['TaskDetail'] = {
  id: taskId,
  key: taskReference.display_key,
  title: taskReference.title,
  project_id: '0193a5b0-4444-7000-8000-000000000001',
  team_id: '0193a5b0-3333-7000-8000-000000000001',
  status: { native_id: '3', name: 'In Progress', meaning: 'active' },
  task_type: 'Story',
  size_label: 'M',
  age_seconds: 518400,
  over_target: true,
  updated_at: '2026-11-07T14:00:00Z',
  source_url: taskReference.source_url,
  target: { duration_seconds: 432000, version_no: 2 },
  blocked_intervals: [],
  observations: [
    {
      kind: 'awaiting_reviewer',
      since: readyAt,
      basis: [mrReference],
      action: 'assign_reviewer',
    },
  ],
  links: [
    {
      id: '0193a5b0-c0c0-7000-8000-000000000001',
      link_type: 'task_mr',
      method: 'source_explicit',
      status: 'confirmed',
      from_object_id: taskId,
      to: mrReference,
      decided_at: null,
    },
  ],
  redactions: [],
}

export const taskPage: Schema['TaskPage'] = {
  data: [taskDetail],
  next_cursor: null,
  access_limited: false,
  access_notes: [],
}

export const readyEvent: Schema['TimelineEvent'] = {
  ...lateTimelineEvent,
  id: '0193a5b0-e1e1-7000-8000-000000000042',
  event_kind: 'ready',
  field: null,
  before_value: null,
  after_value: null,
  event_time: readyAt,
  observed_at: '2026-11-06T12:00:01Z',
  late: false,
  object: {
    ...mrReference,
    source_url: mrReference.source_url + '#note_42',
  },
}

export const taskHistory: Schema['TimelinePage'] = {
  data: [readyEvent],
  history_from: '2026-10-01T00:00:00Z',
  next_cursor: null,
  access_limited: false,
  access_notes: [],
}
