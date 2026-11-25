import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import { readNullableNumber, readScopedPage } from './read-fields'
import {
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

function readRoots(value: unknown): OpenCodes<Schema['DocumentationRootList']> {
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

export function listServiceDocuments(
  api: ApiClient,
  serviceId: operations['listServiceDocuments']['parameters']['path']['service_id'],
  params: ServiceDocumentListParams = {},
) {
  return queryOptions({
    queryKey: ['documentation', 'documents', serviceId, params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/services/{service_id}/documents', readDocumentPage, {
          pathParams: { service_id: serviceId },
          query: params,
          signal,
        })
      ).data,
  })
}

export function listDocumentationRoots(
  api: ApiClient,
  serviceId: operations['listDocumentationRoots']['parameters']['path']['service_id'],
) {
  return queryOptions({
    queryKey: ['documentation', 'roots', serviceId],
    queryFn: ({ signal }) =>
      api.read('/services/{service_id}/documentation-roots', readRoots, {
        pathParams: { service_id: serviceId },
        signal,
      }),
  })
}
