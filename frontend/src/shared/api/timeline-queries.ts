import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import {
  readActor,
  readNumber,
  readObjectRef,
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
export type TaskEventsParams = NonNullable<
  operations['listTaskEvents']['parameters']['query']
>
export type ProjectTimelineParams =
  operations['listProjectTimeline']['parameters']['query']
export type TimelineEvent = OpenCodes<Schema['TimelineEvent']>

function readEvent(value: unknown): TimelineEvent {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    object: readObjectRef(raw.object),
    event_kind: readString(raw.event_kind),
    field: readNullableString(raw.field),
    before_value: readNullableString(raw.before_value),
    after_value: readNullableString(raw.after_value),
    actor: readActor(raw.actor),
    event_time: readString(raw.event_time),
    event_offset_seconds: readNumber(raw.event_offset_seconds),
    observed_at: readString(raw.observed_at),
    observed_via: readString(raw.observed_via),
    late: readBoolean(raw.late),
  }
}

function readTimeline(value: unknown): OpenCodes<Schema['TimelinePage']> {
  const raw = readRecord(value)
  return {
    ...readScopedPage(raw),
    data: readArray(raw.data, readEvent),
    history_from: readNullableString(raw.history_from),
    ...(raw.sources === undefined
      ? {}
      : {
          sources: readArray(raw.sources, (value) => {
            const item = readRecord(value)
            return {
              connection_id: readString(item.connection_id),
              provider: readString(item.provider),
              history_from: readNullableString(item.history_from),
              completeness: readString(item.completeness),
            }
          }),
        }),
  }
}

export function listTaskEvents(
  api: ApiClient,
  taskId: operations['listTaskEvents']['parameters']['path']['task_id'],
  params: TaskEventsParams = {},
) {
  return queryOptions({
    queryKey: ['tasks', 'events', taskId, params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/tasks/{task_id}/events', readTimeline, {
          pathParams: { task_id: taskId },
          query: params,
          signal,
        })
      ).data,
  })
}

export function listProjectTimeline(
  api: ApiClient,
  projectId: operations['listProjectTimeline']['parameters']['path']['project_id'],
  params: ProjectTimelineParams,
) {
  return queryOptions({
    queryKey: ['projects', 'timeline', projectId, params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/projects/{project_id}/timeline', readTimeline, {
          pathParams: { project_id: projectId },
          query: params,
          signal,
        })
      ).data,
  })
}
