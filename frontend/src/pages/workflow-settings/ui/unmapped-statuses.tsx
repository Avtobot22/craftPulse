import type { Catalog } from '../api/workflow-responses'
import { sourceLabel } from '../model/drafts'

export function UnmappedStatuses({
  statuses,
  catalog,
}: {
  statuses: Catalog['unmapped_statuses']
  catalog: Catalog
}) {
  const unique = new Map(
    statuses.map((status) => [
      JSON.stringify([status.instance_id, status.native_id]),
      status,
    ]),
  )
  if (!unique.size) return null
  return (
    <div>
      <h4>Статусы без правила</h4>
      <p>Эти статусы не дают начала и завершения работы.</p>
      <ul>
        {[...unique].map(([key, status]) => {
          const instance = catalog.instances.find(
            (item) => item.instance_id === status.instance_id,
          )
          const name = instance?.statuses.find(
            (item) => item.native_id === status.native_id,
          )?.name
          return (
            <li key={key}>
              {name ? `${name} (${status.native_id})` : status.native_id} ·{' '}
              {instance ? sourceLabel(instance) : status.instance_id}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
