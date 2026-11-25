import { generatePath } from 'react-router'
import type { operations } from '@/shared/api'
import { routePaths } from './routes'

type Query<
  T extends 'listTasks' | 'listProjectTimeline' | 'listMergeRequests',
> = NonNullable<operations[T]['parameters']['query']>

type Scope = Pick<Query<'listTasks'>, 'team_id' | 'project_id'>
type ProjectScope = Scope & { project_id: string }
type TaskFilters = Pick<
  Query<'listTasks'>,
  'status_meaning' | 'over_target' | 'sort' | 'cursor'
>
type TimelineFilters = Pick<
  Query<'listProjectTimeline'>,
  'actor_kind' | 'object_kind' | 'revision_id' | 'cursor'
>
type QueueFilters = Pick<
  Query<'listMergeRequests'>,
  'review_wait' | 'sort' | 'cursor'
>

// date is YYYY-MM-DD in the displayed zone. from/to are RFC 3339 instants.
// When a screen changes a scope, filter or sort, it drops the previous cursor.
type TimelinePeriod =
  | { date: string; from?: never; to?: never }
  | { date?: never; from: string; to: string }

export type TaskListSearch = Scope & TaskFilters
export type TaskDetailSearch = Scope & {
  from?: string
  to?: string
  date?: string
  as_of?: string
  revision_id?: string
}
export type ProjectTimelineSearch = ProjectScope &
  TimelinePeriod &
  TimelineFilters
export type MergeRequestQueueSearch = ProjectScope &
  QueueFilters & {
    from?: string
    to?: string
  }

function route(
  path: string,
  params: Record<string, string | boolean | undefined>,
) {
  const search = new URLSearchParams()
  for (const [name, value] of Object.entries(params)) {
    if (value !== undefined) search.set(name, String(value))
  }
  return search.size ? path + '?' + search.toString() : path
}

export const taskAnalysisLinks = {
  taskList: (params: TaskListSearch) => route(routePaths.taskList, params),
  taskDetail: (taskId: string, params: TaskDetailSearch = {}) =>
    route(
      generatePath(routePaths.taskDetail, {
        taskId: encodeURIComponent(taskId),
      }),
      params,
    ),
  projectTimeline: (params: ProjectTimelineSearch) =>
    route(routePaths.projectTimeline, params),
  mergeRequestQueue: (params: MergeRequestQueueSearch) =>
    route(routePaths.mergeRequestQueue, params),
}
