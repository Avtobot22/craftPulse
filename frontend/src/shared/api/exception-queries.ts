import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
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
    queryFn: ({ signal }) =>
      api.read('/exceptions/{exception_id}', readExceptionDetail, {
        pathParams: { exception_id: exceptionId },
        signal,
      }),
  })
}
