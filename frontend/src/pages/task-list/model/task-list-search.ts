import type { TaskListParams } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'

const meanings = Object.keys(contractLabels.taskStatus)
export const statusOptions = Object.entries(contractLabels.taskStatus).map(
  ([code, label]) => [code, taskStatusLabel(code, label)] as const,
)

export function taskStatusLabel(code: string, label?: string) {
  return code === 'unknown'
    ? 'Статус не сопоставлен'
    : (label ?? contractLabel(contractLabels.taskStatus, code))
}

export type Sort = NonNullable<TaskListParams['sort']>
const sorts: readonly Sort[] = [
  '-age',
  'age',
  '-updated_at',
  'updated_at',
  'key',
  '-key',
]
export const pageSizes = [25, 50, 100] as const

export function readTaskListSearch(search: URLSearchParams): TaskListParams & {
  sort: Sort
  limit: (typeof pageSizes)[number]
} {
  const meaning = meanings.find(
    (value): value is keyof typeof contractLabels.taskStatus =>
      value === search.get('status_meaning'),
  )
  const overTarget = search.get('over_target')
  const cursor = search.get('cursor')
  const team = search.get('team_id')
  const project = search.get('project_id')
  return {
    ...(team ? { team_id: team } : {}),
    ...(project ? { project_id: project } : {}),
    ...(meaning ? { status_meaning: meaning } : {}),
    ...(overTarget === 'true' || overTarget === 'false'
      ? { over_target: overTarget === 'true' }
      : {}),
    sort: sorts.find((value) => value === search.get('sort')) ?? '-age',
    limit: pageSizes.find((size) => String(size) === search.get('limit')) ?? 25,
    ...(cursor ? { cursor } : {}),
  }
}

export function readPreviousCursors(state: unknown): string[] {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('taskListCursors' in state)
  )
    return []
  const cursors: unknown = state.taskListCursors
  return Array.isArray(cursors) &&
    cursors.every((cursor) => typeof cursor === 'string')
    ? cursors
    : []
}
