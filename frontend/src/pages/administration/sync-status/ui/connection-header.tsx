import { Text } from '@gravity-ui/uikit'
import { Link, generatePath } from 'react-router'
import { CapabilityList } from '@/entities/connection'
import type { components, OpenCodes } from '@/shared/api'
import {
  adminLabels,
  contractLabel,
  providerLabel,
  routePaths,
} from '@/shared/config'
import { StatusTime } from '@/shared/ui'

export function ConnectionHeader({
  connection,
  status,
}: {
  connection: OpenCodes<components['schemas']['Connection']>
  status: string
}) {
  const { instance, scopes, capabilities } = connection
  return (
    <header>
      <Link to={routePaths.connections}>Подключения</Link>
      <Text as="h1" variant="header-1">
        Состояние сбора
      </Text>
      <Text as="h2" variant="header-2">
        {connection.name}
      </Text>
      <dl className="cp-sync-status__details">
        <dt>Провайдер</dt>
        <dd>{providerLabel(connection.provider)}</dd>
        <dt>Экземпляр</dt>
        <dd>{instance.base_url ?? instance.id}</dd>
        <dt>Редакция</dt>
        <dd>{instance.edition ?? 'Нет данных'}</dd>
        <dt>Версия источника</dt>
        <dd>{instance.source_version ?? 'Нет данных'}</dd>
        <dt>Статус подключения</dt>
        <dd>{contractLabel(adminLabels.connectionStatus, status)}</dd>
        <dt>Последняя замена учётных данных</dt>
        <dd>
          <StatusTime value={connection.credentials_set_at} />
        </dd>
      </dl>
      {status === 'access_lost' && (
        <div role="status">
          <p>
            Доступ к источнику потерян. Текущие результаты этого источника
            неизвестны.
          </p>
          <dl className="cp-sync-status__details">
            <dt>Дата потери доступа</dt>
            <dd>
              <StatusTime value={connection.access_lost_at ?? null} />
            </dd>
            <dt>Последний успешный снимок</dt>
            <dd>
              <StatusTime value={connection.last_success_at} />
            </dd>
          </dl>
          <Link
            to={generatePath(routePaths.connectionDetail, {
              connectionId: encodeURIComponent(connection.id),
            })}
          >
            Заменить учётные данные
          </Link>
        </div>
      )}
      <Text as="h3" variant="subheader-2">
        Области подключения
      </Text>
      {scopes.length === 0 && <p>Области не указаны</p>}
      {scopes.length > 0 && (
        <ul>
          {scopes.map((scope) => (
            <li key={scope.id}>
              {scope.display_name} · начало истории:{' '}
              <StatusTime value={scope.history_from} />
            </li>
          ))}
        </ul>
      )}
      <Text as="h3" variant="subheader-2">
        Возможности подключения
      </Text>
      {capabilities.length > 0 ? (
        <CapabilityList capabilities={capabilities} />
      ) : (
        <p>Нет данных о возможностях</p>
      )}
    </header>
  )
}
