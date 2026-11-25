import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import { listTasks } from '@/shared/api'
import type { ApiClient, TaskListParams } from '@/shared/api'
import { contractLabels } from '@/shared/config'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  TaskDetailLink,
} from '@/shared/ui'

const sorts = [
  'age',
  '-age',
  'updated_at',
  '-updated_at',
  'key',
  '-key',
] as const

function taskFilters(search: URLSearchParams): TaskListParams {
  const meaning = Object.keys(contractLabels.taskStatus).find(
    (value): value is keyof typeof contractLabels.taskStatus =>
      value === search.get('status_meaning'),
  )
  const sort = sorts.find((value) => value === search.get('sort'))
  const overTarget = search.get('over_target')
  const cursor = search.get('cursor')
  const team = search.get('team_id')
  const project = search.get('project_id')

  return {
    ...(team ? { team_id: team } : {}),
    ...(project ? { project_id: project } : {}),
    ...(meaning ? { status_meaning: meaning } : {}),
    ...(sort ? { sort } : {}),
    ...(cursor ? { cursor } : {}),
    ...(overTarget === 'true' || overTarget === 'false'
      ? { over_target: overTarget === 'true' }
      : {}),
  }
}

export function TaskListPage({ api }: { api: ApiClient }) {
  const [search] = useSearchParams()
  const filters = taskFilters(search)
  const { team_id, project_id } = filters
  const query = useQuery(listTasks(api, filters))
  const next = new URLSearchParams(search)
  if (query.data?.next_cursor) next.set('cursor', query.data.next_cursor)

  return (
    <section>
      <Text as="h1" variant="header-1">
        Задачи
      </Text>
      {query.isPending ? (
        <LoadingState />
      ) : query.isError ? (
        <ErrorState
          reason="Не удалось загрузить список задач"
          retrying={query.isFetching}
          onRetry={() => {
            void query.refetch()
          }}
        />
      ) : (
        <>
          <RestrictedResults
            access_limited={query.data.access_limited}
            access_notes={query.data.access_notes}
          />
          {query.data.data.length > 0 ? (
            <ul>
              {query.data.data.map((task) => (
                <li key={task.id}>
                  <TaskDetailLink
                    taskId={task.id}
                    params={{
                      ...(team_id ? { team_id } : {}),
                      ...(project_id ? { project_id } : {}),
                    }}
                  >
                    {[task.key, task.title].filter(Boolean).join(' · ') ||
                      'Открыть задачу'}
                  </TaskDetailLink>
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState reason="filters" />
          )}
          {query.data.next_cursor && (
            <Link to={{ search: next.toString() }}>Следующая страница</Link>
          )}
        </>
      )}
    </section>
  )
}
