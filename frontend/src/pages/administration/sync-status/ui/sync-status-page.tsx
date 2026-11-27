import { Text } from '@gravity-ui/uikit'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect } from 'react'
import { Link, useOutletContext, useParams } from 'react-router'
import {
  ApiError,
  getConnection,
  getSyncStatus,
  isUnavailableError,
  type ApiClient,
} from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { adminLinks } from '@/shared/config'
import {
  ErrorState,
  LoadingState,
  StatusTime,
  UnavailableRedirect,
} from '@/shared/ui'
import { ConnectionActions } from './connection-actions'
import { ConnectionHeader } from './connection-header'
import { ReconcileSummary } from './reconcile-summary'
import { SyncRuns } from './sync-runs'
import './sync-status-page.css'

const refreshInterval = 5_000

export function SyncStatusPage({ api }: { api: ApiClient }) {
  const { connectionId } = useParams()
  const { actions } = useOutletContext<CurrentUser>()
  if (!connectionId || !actions.includes('connections.manage')) {
    return <UnavailableRedirect forbidden />
  }
  return (
    <SyncStatusContent
      key={connectionId}
      api={api}
      connectionId={connectionId}
      canReadSync={actions.includes('sync.read')}
    />
  )
}

function SyncStatusContent({
  api,
  connectionId,
  canReadSync,
}: {
  api: ApiClient
  connectionId: string
  canReadSync: boolean
}) {
  const client = useQueryClient()
  const connection = useQuery({
    ...getConnection(api, connectionId),
    refetchOnMount: 'always',
    refetchOnWindowFocus: 'always',
  })
  const sync = useQuery({
    ...getSyncStatus(api, connectionId),
    enabled: canReadSync,
    refetchOnMount: 'always',
    refetchOnWindowFocus: 'always',
    refetchInterval: ({ state }) => {
      const data = state.data
      return state.fetchStatus === 'idle' &&
        data?.connection_status === 'active' &&
        ((data.paused_until !== null &&
          Date.parse(data.paused_until) > state.dataUpdatedAt) ||
          data.last_reconcile?.outcome === 'running' ||
          data.data_kinds.some((kind) => kind.phase === 'backfill'))
        ? refreshInterval
        : false
    },
  })
  const refresh = useCallback(async () => {
    const refreshQuery = async (queryKey: readonly string[]) => {
      const inFlight = client.isFetching({ queryKey, exact: true }) > 0
      const invalidate = () =>
        client.invalidateQueries(
          { queryKey, exact: true },
          { cancelRefetch: false },
        )
      await invalidate()
      // If this joined an older GET, read again after it settles.
      if (inFlight) await invalidate()
    }
    await Promise.all([
      refreshQuery(getConnection(api, connectionId).queryKey),
      ...(canReadSync
        ? [refreshQuery(getSyncStatus(api, connectionId).queryKey)]
        : []),
    ])
  }, [api, canReadSync, client, connectionId])

  const resource = connection.data
  const data = canReadSync && !sync.isError ? sync.data : undefined
  const readConnection = connection.refetch
  const connectionStatus = resource?.data.status
  const syncStatus = data?.connection_status
  useEffect(() => {
    // The sync poll can discover revocation before the connection details are reread.
    if (connectionStatus && syncStatus && connectionStatus !== syncStatus) {
      void readConnection({ cancelRefetch: false })
    }
  }, [connectionStatus, readConnection, syncStatus])

  if (isUnavailableError(connection.error))
    return <UnavailableRedirect error={connection.error} />
  const status = data?.connection_status ?? resource?.data.status
  const effectiveStatus =
    status === 'access_lost' || resource?.data.status === 'access_lost'
      ? 'access_lost'
      : status
  const pausedUntil = data
    ? data.paused_until
    : (resource?.data.paused_until ?? null)
  const rateLimited =
    pausedUntil !== null &&
    Date.parse(pausedUntil) >
      (data ? sync.dataUpdatedAt : connection.dataUpdatedAt)
  const connectionCorrelation =
    connection.error instanceof ApiError
      ? connection.error.problem?.correlation_id
      : undefined
  const syncCorrelation =
    sync.error instanceof ApiError
      ? sync.error.problem?.correlation_id
      : undefined

  return (
    <section className="cp-sync-status" aria-label="Состояние сбора">
      {resource && effectiveStatus ? (
        <ConnectionHeader connection={resource.data} status={effectiveStatus} />
      ) : (
        <Text as="h1" variant="header-1">
          Состояние сбора
        </Text>
      )}
      {connection.isPending && <LoadingState />}
      {connection.isError && (
        <ErrorState
          reason="Не удалось загрузить подключение"
          {...(connectionCorrelation
            ? { correlationId: connectionCorrelation }
            : {})}
          retrying={connection.isFetching}
          onRetry={() => void connection.refetch({ cancelRefetch: false })}
        />
      )}
      {rateLimited && effectiveStatus !== 'access_lost' && (
        <p role="status">
          Лимит источника, следующий запрос не раньше{' '}
          <StatusTime value={pausedUntil} />
        </p>
      )}
      {resource?.data.sync_paused && effectiveStatus === 'active' && (
        <p role="status">
          Сбор приостановлен. Уже загруженные данные сохранены.
        </p>
      )}
      {resource && !connection.isError && (
        <ConnectionActions
          api={api}
          resource={resource}
          refresh={refresh}
          status={effectiveStatus}
        />
      )}
      {!canReadSync && <p>Недостаточно прав для чтения состояния сбора</p>}
      {canReadSync && sync.isPending && <LoadingState />}
      {canReadSync && sync.isError && (
        <ErrorState
          reason="Не удалось загрузить состояние сбора"
          {...(syncCorrelation ? { correlationId: syncCorrelation } : {})}
          retrying={sync.isFetching}
          onRetry={() => void sync.refetch({ cancelRefetch: false })}
        />
      )}
      {canReadSync && (
        <SyncRuns
          api={api}
          connectionId={connectionId}
          refresh={refresh}
          kinds={data?.data_kinds ?? []}
          status={effectiveStatus}
          paused={resource?.data.sync_paused ?? false}
          rateLimited={rateLimited}
          reconcileRunning={data?.last_reconcile?.outcome === 'running'}
          canRun={Boolean(resource) && !connection.isError}
        />
      )}
      {data?.data_kinds.length === 0 && (
        <p>Состояние по видам данных ещё не получено</p>
      )}
      {data && <ReconcileSummary reconcile={data.last_reconcile} />}
      <nav aria-label="Диагностика приёма">
        <ul>
          {canReadSync && (
            <li>
              <Link
                to={adminLinks.receiptJournal({
                  connection_id: connectionId,
                  channel: 'sync',
                })}
              >
                Журнал приёма сбора
              </Link>
            </li>
          )}
          <li>
            <Link to={adminLinks.ingestConflicts()}>Конфликты приёма</Link>
          </li>
        </ul>
      </nav>
    </section>
  )
}
