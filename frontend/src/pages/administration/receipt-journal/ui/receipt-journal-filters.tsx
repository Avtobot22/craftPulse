import { useQuery } from '@tanstack/react-query'
import { allConnections, allProjects } from '@/shared/api'
import type { ApiClient, ReceiptListParams } from '@/shared/api'
import { adminLabels, readTechnicalId } from '@/shared/config'

const channels = ['sync', 'ci', 'import'] as const
const statuses = [
  'quarantined',
  'accepted',
  'partial',
  'rejected',
  'duplicate',
] as const

export function readJournalFilters(search: URLSearchParams): ReceiptListParams {
  const channel = channels.find((code) => code === search.get('channel'))
  const status = statuses.find((code) => code === search.get('status'))
  const connection_id = readTechnicalId(search.get('connection_id'))
  const project_id = readTechnicalId(search.get('project_id'))
  return {
    ...(channel ? { channel } : {}),
    ...(status ? { status } : {}),
    ...(connection_id ? { connection_id } : {}),
    ...(project_id ? { project_id } : {}),
  }
}

function ReceiptScopeOptions({
  label,
  all,
  value,
  entries,
  pending,
  failed,
  onChange,
}: {
  label: string
  all: string
  value: string | undefined
  entries: readonly { id: string; name: string }[]
  pending: boolean
  failed: boolean
  onChange: (value: string) => void
}) {
  return (
    <div>
      <label>
        {label}
        <select
          value={value ?? ''}
          onChange={(event) => {
            onChange(event.target.value)
          }}
        >
          <option value="">{all}</option>
          {value && !entries.some(({ id }) => id === value) && (
            <option value={value}>{value}</option>
          )}
          {entries.map(({ id, name }) => (
            <option key={id} value={id}>
              {name}
            </option>
          ))}
        </select>
      </label>
      {pending && <small role="status">Загружаются варианты</small>}
      {failed && <small role="status">Варианты недоступны</small>}
    </div>
  )
}

export function ReceiptJournalFilters({
  api,
  actions,
  identity,
  filters,
  onChange,
}: {
  api: ApiClient
  actions: readonly string[]
  identity: string
  filters: ReceiptListParams
  onChange: (name: string, value: string) => void
}) {
  const canReadConnections = actions.includes('connections.manage')
  const canReadProjects = actions.includes('org.read')
  const connections = useQuery({
    ...allConnections(api),
    enabled: canReadConnections,
  })
  const projects = useQuery({
    ...allProjects(api, identity),
    enabled: canReadProjects,
  })

  return (
    <div className="cp-receipt-journal__filters">
      <label>
        Канал
        <select
          value={filters.channel ?? ''}
          onChange={(event) => {
            onChange('channel', event.target.value)
          }}
        >
          <option value="">Все каналы</option>
          {channels.map((code) => (
            <option key={code} value={code}>
              {adminLabels.receiptChannel[code]}
            </option>
          ))}
        </select>
      </label>
      <label>
        Статус пакета
        <select
          value={filters.status ?? ''}
          onChange={(event) => {
            onChange('status', event.target.value)
          }}
        >
          <option value="">Все статусы</option>
          {statuses.map((code) => (
            <option key={code} value={code}>
              {adminLabels.receiptStatus[code]}
            </option>
          ))}
        </select>
      </label>
      <ReceiptScopeOptions
        label="Подключение"
        all="Все подключения"
        value={filters.connection_id}
        entries={
          canReadConnections && !connections.isError
            ? (connections.data ?? [])
            : []
        }
        pending={canReadConnections && connections.isPending}
        failed={!canReadConnections || connections.isError}
        onChange={(value) => {
          onChange('connection_id', value)
        }}
      />
      <ReceiptScopeOptions
        label="Проект"
        all="Все проекты"
        value={filters.project_id}
        entries={
          canReadProjects && !projects.isError ? (projects.data ?? []) : []
        }
        pending={canReadProjects && projects.isPending}
        failed={!canReadProjects || projects.isError}
        onChange={(value) => {
          onChange('project_id', value)
        }}
      />
    </div>
  )
}
