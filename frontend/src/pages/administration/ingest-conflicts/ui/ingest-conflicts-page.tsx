import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import { useLocation, useOutletContext, useSearchParams } from 'react-router'
import { ApiError, isUnavailableError, listIngestConflicts } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  CursorPagination,
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import { IngestConflictsTable } from './ingest-conflicts-table'
import './ingest-conflicts.css'

function cursorHistory(state: unknown): string[] {
  const cursors =
    state && typeof state === 'object' && 'conflictCursors' in state
      ? state.conflictCursors
      : null
  return Array.isArray(cursors) &&
    cursors.every((cursor) => typeof cursor === 'string')
    ? cursors
    : []
}

function pageSize(value: string | null) {
  if (value === '50') return 50
  if (value === '100') return 100
  return 25
}

export function IngestConflictsPage({ api }: { api: ApiClient }) {
  const { actions } = useOutletContext<CurrentUser>()
  const canRead = actions.includes('sync.read')
  const canResolve = actions.includes('connections.manage')
  const [search, setSearch] = useSearchParams()
  const location = useLocation()
  const heading = useRef<HTMLHeadingElement>(null)
  const status = search.get('status') === 'resolved' ? 'resolved' : 'open'
  const cursor = search.get('cursor') ?? ''
  const limit = pageSize(search.get('limit'))
  const list = useQuery({
    ...listIngestConflicts(api, {
      status,
      limit,
      ...(cursor ? { cursor } : {}),
    }),
    enabled: canRead,
    // Keep resolved rows visible on the page where the administrator acted.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  if (!canRead || isUnavailableError(list.error)) {
    return <UnavailableRedirect error={list.error} forbidden />
  }

  function update(
    nextStatus: 'open' | 'resolved',
    nextCursor: string,
    nextLimit: number,
    conflictCursors: string[] = [],
  ) {
    const params = new URLSearchParams()
    if (nextStatus === 'resolved') params.set('status', nextStatus)
    if (nextCursor) params.set('cursor', nextCursor)
    if (nextLimit !== 25) params.set('limit', String(nextLimit))
    setSearch(params, { state: { conflictCursors } })
  }

  const page = list.isError ? undefined : list.data
  const correlationId =
    list.error instanceof ApiError ? list.error.problem?.correlation_id : null

  return (
    <section
      className="cp-ingest-conflicts"
      aria-labelledby="ingest-conflicts-heading"
    >
      <Text
        as="h1"
        id="ingest-conflicts-heading"
        variant="header-1"
        ref={heading}
        tabIndex={-1}
      >
        Конфликты приёма
      </Text>
      <label className="cp-ingest-conflicts__filter">
        Статус конфликта
        <select
          value={status}
          onChange={(event) => {
            update(
              event.target.value === 'resolved' ? 'resolved' : 'open',
              '',
              limit,
            )
          }}
        >
          <option value="open">Открытые</option>
          <option value="resolved">Решённые</option>
        </select>
      </label>
      {list.isPending && <LoadingState />}
      {list.isError && (
        <ErrorState
          reason="Не удалось загрузить конфликты приёма"
          {...(correlationId ? { correlationId } : {})}
          retrying={list.isFetching}
          onRetry={() => void list.refetch()}
        />
      )}
      {page &&
        (page.data.length ? (
          <IngestConflictsTable
            key={JSON.stringify([status, cursor, limit])}
            api={api}
            conflicts={page.data}
            canResolve={canResolve && status === 'open' && !list.isFetching}
          />
        ) : (
          <EmptyState
            reason="filters"
            title={
              status === 'open'
                ? 'Открытых конфликтов нет'
                : 'Решённых конфликтов нет'
            }
          />
        ))}
      {page &&
        (page.data.length > 0 ||
          Boolean(page.next_cursor) ||
          Boolean(cursor)) && (
          <CursorPagination
            label="Страницы конфликтов"
            cursor={cursor}
            previous={cursor ? cursorHistory(location.state) : []}
            next={page.next_cursor}
            limit={limit}
            received={page.data.length}
            goTo={(targetCursor, history, size) => {
              heading.current?.focus()
              update(status, targetCursor, pageSize(size), history)
            }}
          />
        )}
    </section>
  )
}
