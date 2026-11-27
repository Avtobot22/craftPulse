import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import { useLocation, useSearchParams } from 'react-router'
import { ApiError, listTasks } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import {
  CursorPagination,
  ErrorState,
  LoadingState,
  StaleState,
  UnavailableRedirect,
} from '@/shared/ui'
import {
  readPreviousCursors,
  readTaskListSearch,
} from '../model/task-list-search'
import { TaskFilters } from './task-filters'
import { TaskListResults } from './task-list-results'
import './task-list-page.css'

export function TaskListPage({ api }: { api: ApiClient }) {
  const [search, setSearch] = useSearchParams()
  const location = useLocation()
  const heading = useRef<HTMLHeadingElement>(null)
  const params = readTaskListSearch(search)
  const {
    cursor = '',
    limit,
    sort,
    status_meaning: status = '',
    over_target: overTarget,
    team_id,
    project_id,
  } = params
  const previous = cursor ? readPreviousCursors(location.state) : []
  const deadline = overTarget === undefined ? '' : String(overTarget)
  const scope = {
    ...(team_id ? { team_id } : {}),
    ...(project_id ? { project_id } : {}),
  }

  const query = useQuery({
    ...listTasks(api, params),
    refetchOnMount: 'always',
  })
  const error = query.error instanceof ApiError ? query.error : null
  const correlationId = error?.problem?.correlation_id
  if (error?.kind === 'forbidden' || error?.kind === 'not-found') {
    return <UnavailableRedirect error={error} />
  }
  const waiting =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const stale =
    query.isError &&
    (error?.kind === 'network' || (error?.problem?.status ?? 0) >= 500)
  const page = waiting || (query.isError && !stale) ? undefined : query.data
  const failure = query.isError && (
    <ErrorState
      reason="Не удалось загрузить список задач"
      {...(correlationId ? { correlationId } : {})}
      retrying={query.isFetching}
      onRetry={() => void query.refetch()}
    />
  )

  function update(
    values: Record<string, string>,
    taskListCursors: string[] = [],
  ) {
    const next = new URLSearchParams(search)
    for (const [name, value] of Object.entries(values)) {
      if (value === '') next.delete(name)
      else next.set(name, value)
    }
    heading.current?.focus()
    setSearch(next, { state: { taskListCursors } })
  }

  function apply(status: string, target: string) {
    update({ status_meaning: status, over_target: target, cursor: '' })
  }

  const results = page && (
    <TaskListResults
      page={page}
      scope={scope}
      sort={sort}
      onSort={(nextSort) => {
        update({ sort: nextSort, cursor: '' })
      }}
      {...(status || deadline
        ? {
            resetFilters: () => {
              apply('', '')
            },
          }
        : {})}
    />
  )

  return (
    <section className="cp-task-list" aria-labelledby="task-list-heading">
      <Text
        as="h1"
        id="task-list-heading"
        ref={heading}
        variant="header-1"
        tabIndex={-1}
      >
        Задачи
      </Text>
      <TaskFilters
        key={JSON.stringify([team_id, project_id, status, deadline])}
        status={status}
        overTarget={deadline}
        onApply={apply}
      />
      {waiting ? (
        <LoadingState />
      ) : page && stale ? (
        <StaleState
          snapshotAt={new Date(query.dataUpdatedAt).toISOString()}
          currentError={failure}
        >
          {results}
        </StaleState>
      ) : (
        (results ?? failure)
      )}
      <CursorPagination
        label="Страницы задач"
        cursor={cursor}
        next={stale ? null : (page?.next_cursor ?? null)}
        previous={previous}
        limit={limit}
        received={
          page && !page.access_limited && page.access_notes.length === 0
            ? page.data.length
            : undefined
        }
        goTo={(cursor, cursors, limit) => {
          update({ cursor, limit }, cursors)
        }}
      />
    </section>
  )
}
