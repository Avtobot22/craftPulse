import { Button, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import {
  Link,
  useLocation,
  useNavigate,
  useOutletContext,
  useSearchParams,
} from 'react-router'
import { ApiError, listConnections } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { routePaths } from '@/shared/config'
import {
  CursorPagination,
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import {
  connectionPageSize,
  connectionStatuses,
  connectionStatusLabels,
  readConnectionCursorHistory,
} from '../model/connection-list'
import { ConnectionsTable } from './connections-table'
import './connections-page.css'

export function ConnectionsPage({ api }: { api: ApiClient }) {
  const { actions } = useOutletContext<CurrentUser>()
  const allowed = actions.includes('connections.manage')
  const canReadSync = actions.includes('sync.read')
  const [search, setSearch] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const heading = useRef<HTMLHeadingElement>(null)
  const status = connectionStatuses.find(
    (value) => value === search.get('status'),
  )
  const cursor = search.get('cursor') ?? ''
  const previous = cursor ? readConnectionCursorHistory(location.state) : []
  const query = useQuery({
    ...listConnections(api, {
      limit: connectionPageSize,
      ...(status ? { status } : {}),
      ...(cursor ? { cursor } : {}),
    }),
    enabled: allowed,
    refetchOnMount: 'always',
    refetchOnWindowFocus: 'always',
  })

  const error = query.error instanceof ApiError ? query.error : null
  const correlationId = error?.problem?.correlation_id
  if (!allowed || error?.kind === 'forbidden') {
    return <UnavailableRedirect error={error} forbidden />
  }

  // Keep this page's rows mounted during a background refresh to preserve focus.
  const connections = query.isError
    ? undefined
    : query.data?.data.slice(0, connectionPageSize)
  const nextCursor = connections ? (query.data?.next_cursor ?? null) : null

  function update(
    nextStatus: string,
    nextCursor: string,
    connectionCursors: string[] = [],
  ) {
    const next = new URLSearchParams(search)
    if (nextStatus) next.set('status', nextStatus)
    else next.delete('status')
    if (nextCursor) next.set('cursor', nextCursor)
    else next.delete('cursor')
    setSearch(next, { state: { connectionCursors } })
  }

  const addConnection = () => void navigate(routePaths.connectionNew)

  return (
    <section className="cp-connections" aria-labelledby="connections-heading">
      <header className="cp-connections__header">
        <Text
          as="h1"
          id="connections-heading"
          variant="header-1"
          ref={heading}
          tabIndex={-1}
        >
          Подключения
        </Text>
        <div className="cp-connections__actions">
          <Button view="action" size="xl" onClick={addConnection}>
            Добавить подключение
          </Button>
          {actions.includes('imports.create') && (
            <Link to={routePaths.importPackage}>Импорт</Link>
          )}
          {canReadSync && (
            <Link to={routePaths.receiptJournal}>Журнал приёма</Link>
          )}
        </div>
      </header>
      <label className="cp-connections__filter">
        Статус подключения
        <select
          value={status ?? ''}
          onChange={(event) => {
            update(event.target.value, '')
          }}
        >
          <option value="">Все статусы</option>
          {connectionStatuses.map((value) => (
            <option key={value} value={value}>
              {connectionStatusLabels[value]}
            </option>
          ))}
        </select>
      </label>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить список подключений"
          {...(correlationId ? { correlationId } : {})}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {connections &&
        (connections.length ? (
          <ConnectionsTable
            api={api}
            connections={connections}
            canReadSync={canReadSync}
          />
        ) : status || cursor ? (
          <EmptyState
            reason="filters"
            title={
              cursor
                ? 'На этой странице нет подключений'
                : 'Нет подключений по фильтру'
            }
          />
        ) : (
          <EmptyState
            reason="source"
            action={{ label: 'Добавить подключение', onClick: addConnection }}
          />
        ))}
      {connections?.length || nextCursor || cursor ? (
        <CursorPagination
          label="Страницы подключений"
          cursor={cursor}
          previous={previous}
          next={nextCursor}
          limit={connectionPageSize}
          received={connections?.length}
          fixedSize
          goTo={(targetCursor, history) => {
            heading.current?.focus()
            update(status ?? '', targetCursor, history)
          }}
        />
      ) : null}
    </section>
  )
}
