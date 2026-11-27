import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import { InvalidResponse } from './response'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import { readNumber, readScopedPage } from './read-fields'
import {
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type ExceptionListParams = NonNullable<
  operations['listExceptions']['parameters']['query']
>

export type ExceptionVersion = OpenCodes<Schema['ExceptionVersion']>
export type ExceptionDetail = OpenCodes<Schema['ExceptionDetail']>
export type ExceptionDraft = Schema['ExceptionDraft']
export type ExceptionDecision = Schema['ExceptionDecision']
export type ExceptionScope = Schema['ExceptionScope']

function readExceptionVersion(
  value: unknown,
): OpenCodes<Schema['ExceptionVersion']> {
  const raw = readRecord(value)
  const scope = readRecord(raw.scope)
  return {
    version_no: readNumber(raw.version_no),
    decision_type: readString(raw.decision_type),
    reason: readString(raw.reason),
    scope: {
      area_id: readString(scope.area_id),
      component_id: readNullableString(scope.component_id),
      finding_id: readNullableString(scope.finding_id),
      rule_id: readNullableString(scope.rule_id),
      path_prefix: readNullableString(scope.path_prefix),
      package_purl: readNullableString(scope.package_purl),
    },
    effective_from: readString(raw.effective_from),
    expires_at: readString(raw.expires_at),
    approval_required: readBoolean(raw.approval_required),
    external_authority: readNullableString(raw.external_authority),
    external_ref: readNullableString(raw.external_ref),
    author_user_id: readString(raw.author_user_id),
    status: readString(raw.status),
    created_at: readString(raw.created_at),
  }
}

function readException(
  raw: Record<string, unknown>,
): OpenCodes<Schema['ExceptionSummary']> {
  return {
    id: readString(raw.id),
    state: readString(raw.state),
    current_version: readExceptionVersion(raw.current_version),
  }
}

function readExceptionPage(value: unknown): OpenCodes<Schema['ExceptionPage']> {
  const raw = readRecord(value)
  return {
    ...readScopedPage(raw),
    data: readArray(raw.data, (item) => readException(readRecord(item))),
  }
}

function readExceptionDetail(
  value: unknown,
): OpenCodes<Schema['ExceptionDetail']> {
  const raw = readRecord(value)
  return {
    ...readException(raw),
    versions: readArray(raw.versions, readExceptionVersion),
    transitions: readArray(raw.transitions, (value) => {
      const transition = readRecord(value)
      return {
        version_no: readNumber(transition.version_no),
        status: readString(transition.status),
        actor_user_id: readString(transition.actor_user_id),
        occurred_at: readString(transition.occurred_at),
      }
    }),
  }
}

export function listExceptions(
  api: ApiClient,
  params: ExceptionListParams = {},
) {
  return queryOptions({
    queryKey: ['exceptions', 'list', params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/exceptions', readExceptionPage, {
          query: params,
          signal,
        })
      ).data,
  })
}

export function getException(
  api: ApiClient,
  exceptionId: operations['getException']['parameters']['path']['exception_id'],
) {
  return queryOptions({
    queryKey: ['exceptions', 'detail', exceptionId],
    queryFn: ({ signal }) => readExceptionById(api, exceptionId, signal),
  })
}

export function readExceptionById(
  api: ApiClient,
  exceptionId: string,
  signal: AbortSignal,
) {
  return api.read('/exceptions/{exception_id}', exceptionDecoder(exceptionId), {
    pathParams: { exception_id: exceptionId },
    signal,
  })
}

function exceptionDecoder(expectedId: string) {
  return (raw: unknown) => {
    const detail = readExceptionDetail(raw)
    if (detail.id !== expectedId) throw new InvalidResponse()
    return detail
  }
}

export function createException(
  api: ApiClient,
  draft: ExceptionDraft,
  idempotencyKey: string,
  signal: AbortSignal,
) {
  return api.createException(draft, readExceptionDetail, {
    idempotencyKey,
    signal,
  })
}

export function createExceptionVersion(
  api: ApiClient,
  exceptionId: string,
  draft: ExceptionDraft,
  ifMatch: string,
  idempotencyKey: string,
  signal: AbortSignal,
) {
  return api.update(
    'POST',
    '/exceptions/{exception_id}/versions',
    draft,
    exceptionDecoder(exceptionId),
    {
      pathParams: { exception_id: exceptionId },
      ifMatch,
      idempotencyKey,
      signal,
    },
  )
}

export function decideException(
  api: ApiClient,
  exceptionId: string,
  decision: ExceptionDecision,
  ifMatch: string,
  idempotencyKey: string,
  signal: AbortSignal,
) {
  return api.update(
    'POST',
    '/exceptions/{exception_id}/decisions',
    decision,
    exceptionDecoder(exceptionId),
    {
      pathParams: { exception_id: exceptionId },
      ifMatch,
      idempotencyKey,
      signal,
    },
  )
}
