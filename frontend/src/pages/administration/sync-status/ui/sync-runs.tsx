import { Button, Flex } from '@gravity-ui/uikit'
import { SyncDataKind } from '@/entities/sync'
import {
  ApiError,
  type ApiClient,
  type components,
  type OpenCodes,
} from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'
import { ErrorState } from '@/shared/ui'
import { useSyncRun } from '../model/use-sync-run'

type Schema = components['schemas']

function isDataKind(kind: string): kind is Schema['DataKind'] {
  return Object.hasOwn(adminLabels.dataKind, kind)
}

const accessLost =
  'Сбор недоступен. Для восстановления доступа замените учётные данные.'
const runFailure = {
  access_lost: accessLost,
  source_unavailable:
    'Источник недоступен. Повторите запуск после восстановления.',
}

export function SyncRuns({
  api,
  connectionId,
  refresh,
  kinds,
  status,
  paused,
  rateLimited,
  reconcileRunning,
  canRun,
}: {
  api: ApiClient
  connectionId: string
  refresh: () => Promise<void>
  kinds: OpenCodes<Schema['SyncDataKindStatus']>[]
  status: string | undefined
  paused: boolean
  rateLimited: boolean
  reconcileRunning: boolean
  canRun: boolean
}) {
  const { start, busy, result, operation, requestError } = useSyncRun(
    api,
    connectionId,
    refresh,
  )
  const conflict =
    requestError instanceof ApiError &&
    requestError.problem?.status === 409 &&
    requestError.problem.code === 'state_conflict'
  const blocked =
    status === 'access_lost'
      ? accessLost
      : status !== 'active'
        ? 'Сбор доступен после активации подключения.'
        : paused
          ? 'Сбор приостановлен. Сначала возобновите сбор.'
          : rateLimited
            ? 'Повтор будет доступен после паузы из-за лимита источника.'
            : busy || reconcileRunning
              ? 'Попытка уже выполняется.'
              : null

  return (
    <>
      {requestError && status !== 'access_lost' && (
        <p role="alert">
          {conflict ? accessLost : 'Не удалось запросить запуск сбора.'}
        </p>
      )}
      {operation.isError && (
        <ErrorState
          reason="Не удалось узнать исход запрошенной попытки"
          retrying={operation.isFetching}
          onRetry={() => void operation.refetch({ cancelRefetch: false })}
        />
      )}
      {result?.status === 'queued' && (
        <p role="status">Попытка ожидает запуска</p>
      )}
      {result?.status === 'running' && <p role="status">Попытка выполняется</p>}
      {result?.status === 'failed' && (
        <p role="alert">
          Запрошенная попытка завершилась ошибкой.{' '}
          {result.error ? contractLabel(runFailure, result.error.code) : null}{' '}
          {result.error?.correlation_id}
        </p>
      )}
      {blocked && (kinds.length > 0 || status === 'access_lost') && (
        <p>{blocked}</p>
      )}
      {kinds.map((kind) => {
        const dataKind = kind.data_kind
        const label = contractLabel(adminLabels.dataKind, dataKind)
        const disabled = Boolean(blocked) || !canRun || !isDataKind(dataKind)
        return (
          <article
            key={dataKind}
            aria-label={label}
            className="cp-sync-status__card"
          >
            <SyncDataKind data={kind} />
            {(kind.last_error_code === 'unauthorized' ||
              kind.last_error_code === 'forbidden') &&
              status !== 'access_lost' && (
                <p>
                  Перед повтором проверьте учётные данные и права чтения в
                  источнике.
                </p>
              )}
            {kind.last_error_code === 'timeout' &&
              !kind.failed_page_present && (
                <p>
                  После восстановления источника продолжите сбор с сохранённой
                  точки.
                </p>
              )}
            <Flex wrap gap="spacing-3">
              <Button
                disabled={disabled}
                onClick={() => {
                  if (!disabled && isDataKind(dataKind))
                    start({ kind: 'resume', data_kind: dataKind })
                }}
              >
                Продолжить сбор
              </Button>
              <Button
                disabled={disabled}
                onClick={() => {
                  if (!disabled && isDataKind(dataKind))
                    start({ kind: 'reconcile', data_kind: dataKind })
                }}
              >
                Сверить с источником
              </Button>
            </Flex>
          </article>
        )
      })}
    </>
  )
}
