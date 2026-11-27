import { Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Link, generatePath, useOutletContext, useParams } from 'react-router'
import { SyncDataKind } from '@/entities/sync'
import { getConnection, getSyncStatus, isUnavailableError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { readTechnicalId, routePaths } from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'

export function SyncStatusPage({ api }: { api: ApiClient }) {
  const { actions } = useOutletContext<CurrentUser>()
  const id = readTechnicalId(useParams().connectionId)
  const canReadSync = actions.includes('sync.read')
  const canManage = actions.includes('connections.manage')
  const connection = useQuery({
    ...getConnection(api, id ?? ''),
    enabled: Boolean(id && canManage),
  })
  const sync = useQuery({
    ...getSyncStatus(api, id ?? ''),
    enabled: Boolean(id && canReadSync),
  })

  if (!id || (!canReadSync && !canManage))
    return <UnavailableRedirect forbidden />
  const accessLost =
    (sync.data?.connection_status ?? connection.data?.data.status) ===
    'access_lost'
  const error = canReadSync ? sync.error : connection.error
  if (isUnavailableError(error)) return <UnavailableRedirect error={error} />

  return (
    <Flex as="section" direction="column" gap="spacing-4">
      <Text as="h1" variant="header-1">
        Состояние сбора
      </Text>
      {accessLost && (
        <div role="status">
          <p>
            Доступ к источнику потерян. Для возобновления сбора замените учётные
            данные.
          </p>
          {canManage && (
            <Link
              to={generatePath(routePaths.connectionDetail, {
                connectionId: id,
              })}
            >
              Восстановить доступ
            </Link>
          )}
        </div>
      )}
      {error ? (
        <ErrorState
          reason="Не удалось загрузить состояние сбора"
          onRetry={() =>
            void (canReadSync ? sync.refetch() : connection.refetch())
          }
        />
      ) : canReadSync ? (
        sync.isPending ? (
          <LoadingState />
        ) : sync.data?.data_kinds.length ? (
          sync.data.data_kinds.map((data) => (
            <SyncDataKind key={data.data_kind} data={data} />
          ))
        ) : (
          <p>Нет данных о сборе</p>
        )
      ) : (
        <p>Недостаточно прав для чтения состояния сбора.</p>
      )}
    </Flex>
  )
}
