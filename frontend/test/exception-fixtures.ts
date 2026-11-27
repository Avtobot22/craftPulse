import type { components } from '../src/shared/api'
import { secretDetail } from './finding-fixtures'
import { currentUser } from './fixtures'

type Schema = components['schemas']

export const areaId = '0193a5b0-4444-7000-8000-000000000001'
export const finding = {
  ...secretDetail,
  area_id: areaId,
  actions: ['create_exception', 'review_exception'],
} satisfies Schema['FindingDetail']

export const authorId = currentUser.user.id
export const reviewerId = '0193a5b0-1111-7000-8000-000000000002'

export const draft = {
  decision_type: 'accepted_risk',
  reason: 'Ротация запланирована на CORE-77',
  scope: {
    area_id: areaId,
    finding_id: finding.id,
    component_id: finding.component_id,
    rule_id: null,
    path_prefix: null,
    package_purl: null,
  },
  effective_from: '2026-01-01T10:00:00Z',
  expires_at: '2099-12-08T12:00:00Z',
} satisfies Schema['ExceptionDraft']

const version: Schema['ExceptionVersion'] = {
  ...draft,
  version_no: 1,
  status: 'pending',
  author_user_id: authorId,
  approval_required: true,
  external_authority: null,
  external_ref: null,
  created_at: draft.effective_from,
}

export const pending: Schema['ExceptionDetail'] = {
  id: '0193a5b0-e3e3-7000-8000-000000000001',
  state: 'pending',
  current_version: version,
  versions: [],
  transitions: [
    {
      version_no: 1,
      status: 'pending',
      actor_user_id: authorId,
      occurred_at: draft.effective_from,
    },
  ],
}

export const approved: Schema['ExceptionDetail'] = {
  ...pending,
  state: 'active',
  current_version: { ...version, status: 'approved' },
  transitions: [
    ...pending.transitions,
    {
      version_no: 1,
      status: 'approved',
      actor_user_id: reviewerId,
      occurred_at: '2026-01-02T12:00:00Z',
    },
  ],
}

export const renewal: Schema['ExceptionDetail'] = {
  ...approved,
  current_version: { ...version, version_no: 2, reason: 'Новая причина' },
  versions: [approved.current_version],
}

export const expired: Schema['ExceptionDetail'] = {
  ...approved,
  state: 'expired',
  current_version: {
    ...approved.current_version,
    effective_from: '2025-10-01T10:00:00Z',
    expires_at: '2026-01-01T10:00:00Z',
  },
}
