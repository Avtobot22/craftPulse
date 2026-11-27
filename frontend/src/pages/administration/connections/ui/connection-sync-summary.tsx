import { useQuery } from '@tanstack/react-query'
import { SyncDataKind } from '@/entities/sync'
import { ApiError, getSyncStatus } from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import { ErrorState, StatusTime } from '@/shared/ui'

export function ConnectionSyncSummary({
  api,
  connection,
  canReadSync,
}: {
  api: ApiClient
  connection: OpenCodes<components['schemas']['Connection']>
  canReadSync: boolean
}) {
  const query = useQuery({
    ...getSyncStatus(api, connection.id),
    enabled: canReadSync,
    refetchOnMount: 'always',
    refetchOnWindowFocus: 'always',
  })
  const sync = canReadSync ? query.data : undefined
  const pausedUntil = sync ? sync.paused_until : connection.paused_until
  const accessLost =
    (sync?.connection_status ?? connection.status) === 'access_lost'
  const noHistory =
    connection.status === 'draft' &&
    connection.last_success_at === null &&
    !sync?.data_kinds.some((kind) => kind.last_success_at !== null)
  const correlationId =
    query.error instanceof ApiError
      ? query.error.problem?.correlation_id
      : undefined

  return (
    <div className="cp-connections__summary">
      {accessLost && (
        <div>Доступ к источнику потерян, нужна замена учётных данных</div>
      )}
      {connection.sync_paused && <div>Сбор остановлен администратором</div>}
      {pausedUntil && (
        <div>
          Лимит источника, следующий запрос не раньше{' '}
          <StatusTime value={pausedUntil} />
        </div>
      )}
      {noHistory && <div>История ещё не получена</div>}
      {canReadSync && query.isPending && <div>Загрузка состояния сбора</div>}
      {!canReadSync && <div>Недостаточно прав для чтения состояния сбора</div>}
      {canReadSync && query.isError && (
        <ErrorState
          reason="Не удалось загрузить состояние сбора"
          {...(correlationId ? { correlationId } : {})}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {sync?.data_kinds.map((kind) => (
        <SyncDataKind key={kind.data_kind} data={kind} summary />
      ))}
      {sync && !sync.data_kinds.length && !noHistory && (
        <div>Нет данных о сборе</div>
      )}
    </div>
  )
}
