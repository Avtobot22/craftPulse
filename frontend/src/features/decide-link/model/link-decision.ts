import type { RefObject } from 'react'
import { ApiError } from '@/shared/api'
import type { components } from '@/shared/api'

export interface LinkDecisionSelection {
  linkId: string
  targetLabel: string
  body: components['schemas']['LinkDecision']
  idempotencyKey: string
  openerRef: RefObject<HTMLElement>
}

export function decisionFailure(error: unknown) {
  if (!(error instanceof ApiError)) return 'uncertain'
  if (error.problem?.status === 409 && error.problem.code === 'state_conflict')
    return 'conflict'
  if (error.kind === 'auth-required' || error.kind === 'session-ended')
    return 'session-ended'
  if (error.kind === 'not-found' || error.kind === 'forbidden')
    return error.kind
  if (error.problem && error.problem.status < 500) return 'rejected'
  return 'uncertain'
}
