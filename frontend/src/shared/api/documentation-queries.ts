import { infiniteQueryOptions, queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import { operationPolling } from './operation-polling'
import type { OpenCodes } from './read-fields'
import {
  readCursorPage,
  readNullableNumber,
  readScopedPage,
} from './read-fields'
import {
  InvalidResponse,
  readArray,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type ServiceDocumentListParams = NonNullable<
  operations['listServiceDocuments']['parameters']['query']
>

function readDocumentPage(value: unknown): OpenCodes<Schema['DocumentPage']> {
  const raw = readRecord(value)
  return {
    ...readScopedPage(raw),
    documentation_state: readString(raw.documentation_state),
    data: readArray(raw.data, (value) => {
      const doc = readRecord(value)
      return {
        id: readString(doc.id),
        title: readNullableString(doc.title),
        doc_format: readString(doc.doc_format),
        path: readNullableString(doc.path),
        version_ref: readString(doc.version_ref),
        last_checked_at: readNullableString(doc.last_checked_at),
        open_violations: readNullableNumber(doc.open_violations),
        source_url: readNullableString(doc.source_url),
      }
    }),
  }
}

export type DocumentationRoots = OpenCodes<Schema['DocumentationRootList']>
export type DocumentationRoot = DocumentationRoots['data'][number]
export type DocumentationRootInput =
  Schema['DocumentationRootListInput']['data'][number]
export type ServiceDocument = OpenCodes<Schema['Document']>

function readRoots(value: unknown): DocumentationRoots {
  return {
    data: readArray(readRecord(value).data, (value) => {
      const root = readRecord(value)
      return {
        id: readString(root.id),
        area_id: readString(root.area_id),
        path_prefix: readString(root.path_prefix),
        profile_id: readString(root.profile_id),
        owner_person_id: readNullableString(root.owner_person_id),
        audience: readString(root.audience),
      }
    }),
  }
}

async function readServiceDocuments(
  api: ApiClient,
  serviceId: string,
  query: ServiceDocumentListParams,
  signal: AbortSignal,
) {
  return (
    await api.read('/services/{service_id}/documents', readDocumentPage, {
      pathParams: { service_id: serviceId },
      query,
      signal,
    })
  ).data
}

export function listServiceDocuments(
  api: ApiClient,
  serviceId: operations['listServiceDocuments']['parameters']['path']['service_id'],
  params: ServiceDocumentListParams = {},
) {
  return queryOptions({
    queryKey: ['documentation', 'documents', serviceId, params],
    queryFn: ({ signal }) =>
      readServiceDocuments(api, serviceId, params, signal),
  })
}

export function listDocumentationRoots(
  api: ApiClient,
  serviceId: operations['listDocumentationRoots']['parameters']['path']['service_id'],
) {
  return queryOptions({
    queryKey: ['documentation', 'roots', serviceId],
    queryFn: ({ signal }) => readDocumentationRoots(api, serviceId, signal),
  })
}

export function serviceDocumentPages(api: ApiClient, serviceId: string) {
  return infiniteQueryOptions({
    queryKey: ['documentation', 'document-pages', serviceId],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) =>
      readServiceDocuments(
        api,
        serviceId,
        pageParam ? { cursor: pageParam } : {},
        signal,
      ),
    getNextPageParam: ({ next_cursor }, _pages, _cursor, cursors) =>
      next_cursor && !cursors.includes(next_cursor) ? next_cursor : undefined,
  })
}

export function readDocumentationRoots(
  api: ApiClient,
  serviceId: string,
  signal: AbortSignal,
) {
  return api.read('/services/{service_id}/documentation-roots', readRoots, {
    pathParams: { service_id: serviceId },
    signal,
  })
}

export function replaceDocumentationRoots(
  api: ApiClient,
  serviceId: string,
  data: DocumentationRootInput[],
  ifMatch: string,
  idempotencyKey: string,
  signal: AbortSignal,
) {
  return api.update(
    'PUT',
    '/services/{service_id}/documentation-roots',
    { data },
    readRoots,
    {
      pathParams: { service_id: serviceId },
      ifMatch,
      idempotencyKey,
      signal,
    },
  )
}

function readDocumentationOperation(value: unknown) {
  const raw = readRecord(value)
  const status = readString(raw.status)
  const error = raw.error === null ? null : readRecord(raw.error)
  const id = readString(raw.id)
  if (
    raw.kind !== 'documentation_check' ||
    !/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu.test(id) ||
    !['queued', 'running', 'succeeded', 'failed'].includes(status)
  )
    throw new InvalidResponse()

  return {
    id,
    status,
    error: error && { code: readString(error.code) },
    retry_after_seconds: readNullableNumber(raw.retry_after_seconds),
  }
}

export async function requestDocumentationCheck(
  api: ApiClient,
  serviceId: string,
  rootId: string,
) {
  return (
    await api.requestDocumentationCheck(
      serviceId,
      { root_id: rootId },
      readDocumentationOperation,
    )
  ).data
}

export function getDocumentationCheck(api: ApiClient, operationId: string) {
  return queryOptions({
    queryKey: ['documentation', 'check', operationId],
    ...operationPolling('queued', 'running'),
    queryFn: async ({ signal }) => {
      const { data } = await api.read(
        '/operations/{operation_id}',
        readDocumentationOperation,
        {
          pathParams: { operation_id: operationId },
          signal,
        },
      )
      if (data.id !== operationId) throw new InvalidResponse()
      return data
    },
  })
}

export function repositoryAreas(api: ApiClient) {
  return queryOptions({
    queryKey: ['documentation', 'repository-areas'],
    queryFn: async ({ signal }) => {
      const areas = new Map<string, string>()
      const cursors = new Set<string>()
      let cursor: string | undefined
      do {
        const { data: page } = await api.read(
          '/areas',
          (value) =>
            readCursorPage(value, (item) => {
              const raw = readRecord(item)
              if (raw.kind !== 'repository') throw new InvalidResponse()
              return {
                id: readString(raw.id),
                name: readString(raw.display_name),
              }
            }),
          { query: { kind: 'repository', cursor }, signal },
        )
        for (const { id, name } of page.data) areas.set(id, name)
        cursor = page.next_cursor ?? undefined
        if (cursor && cursors.has(cursor)) throw new InvalidResponse()
        if (cursor) cursors.add(cursor)
      } while (cursor)
      return areas
    },
  })
}
