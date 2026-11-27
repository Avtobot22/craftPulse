import { generatePath, Link } from 'react-router'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import { contractLabel, providerLabel, routePaths } from '@/shared/config'
import { StatusTime } from '@/shared/ui'
import { connectionStatusLabels } from '../model/connection-list'
import { ConnectionSyncSummary } from './connection-sync-summary'

type Connection = OpenCodes<components['schemas']['Connection']>

export function ConnectionsTable({
  api,
  connections,
  canReadSync,
}: {
  api: ApiClient
  connections: readonly Connection[]
  canReadSync: boolean
}) {
  return (
    <div
      className="cp-connections__scroll"
      role="region"
      aria-label="Таблица подключений"
      tabIndex={0}
    >
      <table className="cp-connections__table" aria-label="Подключения">
        <thead>
          <tr>
            <th scope="col">Подключение</th>
            <th scope="col">Статус</th>
            <th scope="col">Источник</th>
            <th scope="col">Области</th>
            <th scope="col">Последний успешный сбор</th>
            <th scope="col">Состояние сбора</th>
          </tr>
        </thead>
        <tbody>
          {connections.map((connection) => {
            const {
              id,
              name,
              status,
              provider,
              instance,
              scopes,
              last_success_at,
            } = connection
            const path =
              status === 'draft'
                ? routePaths.connectionDetail
                : routePaths.syncStatus
            return (
              <tr key={id}>
                <th scope="row">
                  <Link
                    to={generatePath(path, {
                      connectionId: encodeURIComponent(id),
                    })}
                  >
                    {name}
                  </Link>
                </th>
                <td>{contractLabel(connectionStatusLabels, status)}</td>
                <td>
                  <div>{providerLabel(provider)}</div>
                  <div>{instance.base_url ?? 'Адрес не указан'}</div>
                  <div>Редакция: {instance.edition ?? 'нет данных'}</div>
                  <div>Версия: {instance.source_version ?? 'нет данных'}</div>
                </td>
                <td>
                  {scopes.length
                    ? scopes.map(({ display_name }) => display_name).join(', ')
                    : 'Не заданы'}
                </td>
                <td>
                  <StatusTime value={last_success_at} />
                </td>
                <td>
                  <ConnectionSyncSummary
                    api={api}
                    connection={connection}
                    canReadSync={canReadSync}
                  />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
