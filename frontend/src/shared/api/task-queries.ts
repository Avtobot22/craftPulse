import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import {
  readLink,
  readNullableNumber,
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
export type TaskListParams = NonNullable<
  operations['listTasks']['parameters']['query']
>

function readTaskSummary(
  raw: Record<string, unknown>,
): OpenCodes<Schema['TaskSummary']> {
  const status = readRecord(raw.status)
  return {
    id: readString(raw.id),
    key: readNullableString(raw.key),
    title: readNullableString(raw.title),
    project_id: readNullableString(raw.project_id),
    status: {
      native_id: readString(status.native_id),
      name: readString(status.name),
      meaning: readString(status.meaning),
    },
    task_type: readNullableString(raw.task_type),
    size_label: readNullableString(raw.size_label),
    team_id: readNullableString(raw.team_id),
    age_seconds: readNullableNumber(raw.age_seconds),
    over_target: raw.over_target === null ? null : readBoolean(raw.over_target),
    ...(raw.target_configured === undefined
      ? {}
      : { target_configured: readBoolean(raw.target_configured) }),
    updated_at: readNullableString(raw.updated_at),
    source_url: readNullableString(raw.source_url),
  }
}

function readTaskPage(value: unknown): OpenCodes<Schema['TaskPage']> {
  const raw = readRecord(value)
  return {
    ...readScopedPage(raw),
    ...(raw.empty_reason === undefined
      ? {}
      : { empty_reason: readString(raw.empty_reason) }),
    data: readArray(raw.data, (item) => readTaskSummary(readRecord(item))),
  }
}

function readTaskDetail(value: unknown): OpenCodes<Schema['TaskDetail']> {
  const raw = readRecord(value)
  const target = raw.target === null ? null : readRecord(raw.target)
  return {
    ...readTaskSummary(raw),
    target: target && {
      duration_seconds: readNumber(target.duration_seconds),
      version_no: readNumber(target.version_no),
    },
    blocked_intervals: readArray(raw.blocked_intervals, (value) => {
      const item = readRecord(value)
      return { from: readString(item.from), to: readNullableString(item.to) }
    }),
    observations: readArray(raw.observations, (value) => {
      const item = readRecord(value)
      return {
        kind: readString(item.kind),
        since: readNullableString(item.since),
        basis: readArray(item.basis, readObjectRef),
        action: readString(item.action),
        ...(item.question === undefined
          ? {}
          : { question: readString(item.question) }),
      }
    }),
    links: readArray(raw.links, readLink),
    redactions: readArray(raw.redactions, readString),
  }
}

export function listTasks(api: ApiClient, params: TaskListParams = {}) {
  return queryOptions({
    queryKey: ['tasks', 'list', params],
    queryFn: async ({ signal }) =>
      (await api.read('/tasks', readTaskPage, { query: params, signal })).data,
  })
}

export function getTask(
  api: ApiClient,
  taskId: operations['getTask']['parameters']['path']['task_id'],
) {
  return queryOptions({
    queryKey: ['tasks', 'detail', taskId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/tasks/{task_id}', readTaskDetail, {
          pathParams: { task_id: taskId },
          signal,
        })
      ).data,
  })
}
