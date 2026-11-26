import { RestrictedResults } from '@/entities/result-access'
import type { components, OpenCodes } from '@/shared/api'
import type { TaskDetailSearch } from '@/shared/config'
import { EmptyState } from '@/shared/ui'
import type { Sort } from '../model/task-list-search'
import { TaskTable } from './task-table'

export function TaskListResults({
  page,
  scope,
  sort,
  onSort,
  resetFilters,
}: {
  page: OpenCodes<components['schemas']['TaskPage']>
  scope: TaskDetailSearch
  sort: Sort
  onSort: (sort: Sort) => void
  resetFilters?: () => void
}) {
  const limited = page.access_limited || page.access_notes.length > 0
  const sourceLost = page.access_notes.some(
    ({ code }) => code === 'source_access_lost',
  )
  return (
    <>
      <RestrictedResults access_limited={limited} />
      {sourceLost && (
        <p role="status">Подключение к источнику потеряло доступ.</p>
      )}
      {page.data.length ? (
        <TaskTable
          tasks={page.data}
          scope={scope}
          sort={sort}
          onSort={onSort}
        />
      ) : limited ? (
        <p role="status">На странице нет доступных задач.</p>
      ) : page.empty_reason === 'source_not_configured' ? (
        <EmptyState reason="source" />
      ) : page.empty_reason === 'history_not_received' ? (
        <EmptyState reason="history" />
      ) : (
        <EmptyState
          reason="filters"
          title="Нет задач по фильтрам"
          {...(resetFilters
            ? { action: { label: 'Сбросить фильтры', onClick: resetFilters } }
            : {})}
        />
      )}
    </>
  )
}
