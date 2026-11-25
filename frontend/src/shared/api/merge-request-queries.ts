import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import {
  readAccessNotes,
  readActor,
  readLink,
  readNullableNumber,
  readNumber,
  readScopedPage,
} from './read-fields'
import {
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type MergeRequestListParams = NonNullable<
  operations['listMergeRequests']['parameters']['query']
>
export type ReviewWait = OpenCodes<Schema['ReviewWait']>

function readReviewWait(value: unknown): ReviewWait {
  if (value === null) return null
  const raw = readRecord(value)
  return {
    kind: readString(raw.kind),
    since: readString(raw.since),
    wait_seconds: readNumber(raw.wait_seconds),
    fallback_open: readBoolean(raw.fallback_open),
  }
}

function readMergeRequest(value: unknown): OpenCodes<Schema['MergeRequest']> {
  const raw = readRecord(value)
  const area = readRecord(raw.area)
  return {
    id: readString(raw.id),
    iid: readNullableNumber(raw.iid),
    title: readNullableString(raw.title),
    state: readString(raw.state),
    draft: readBoolean(raw.draft),
    area: {
      id: readString(area.id),
      kind: readString(area.kind),
      display_name: readString(area.display_name),
      connection_id: readNullableString(area.connection_id),
    },
    author: readActor(raw.author),
    head_sha: readString(raw.head_sha),
    ...(raw.base_sha === undefined
      ? {}
      : { base_sha: readNullableString(raw.base_sha) }),
    ...(raw.access_limited === undefined
      ? {}
      : { access_limited: readBoolean(raw.access_limited) }),
    ...(raw.access_notes === undefined
      ? {}
      : { access_notes: readAccessNotes(raw.access_notes) }),
    opened_at: readString(raw.opened_at),
    ready_at: readNullableString(raw.ready_at),
    review_wait: readReviewWait(raw.review_wait),
    source_url: readNullableString(raw.source_url),
    ...(raw.links === undefined
      ? {}
      : { links: readArray(raw.links, readLink) }),
  }
}

function readMergeRequestPage(
  value: unknown,
): OpenCodes<Schema['MergeRequestPage']> {
  const raw = readRecord(value)
  return {
    ...readScopedPage(raw),
    data: readArray(raw.data, readMergeRequest),
  }
}

export function listMergeRequests(
  api: ApiClient,
  params: MergeRequestListParams = {},
) {
  return queryOptions({
    queryKey: ['merge-requests', 'list', params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/merge-requests', readMergeRequestPage, {
          query: params,
          signal,
        })
      ).data,
  })
}

export function getMergeRequest(
  api: ApiClient,
  mrId: operations['getMergeRequest']['parameters']['path']['merge_request_id'],
) {
  return queryOptions({
    queryKey: ['merge-requests', 'detail', mrId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/merge-requests/{merge_request_id}', readMergeRequest, {
          pathParams: { merge_request_id: mrId },
          signal,
        })
      ).data,
  })
}
