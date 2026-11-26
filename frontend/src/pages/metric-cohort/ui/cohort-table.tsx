import { Fragment, useId, useState } from 'react'
import { Button } from '@gravity-ui/uikit'
import { SourceLink } from '@/entities/activity'
import { formatValue, metricReason } from '@/entities/metric'
import type { ApiClient } from '@/shared/api'
import { roleLabels } from '../api/cohort-api'
import type { CohortItem } from '../api/cohort-api'
import { CohortEvidence, ContributionInterval } from './cohort-evidence'

const exclusionReasons: Readonly<Record<string, string>> = {
  cancelled: 'Задача отменена',
  duplicate: 'Задача закрыта как дубликат',
}

const seconds = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 9 })

function contribution(value: number | null, unit: string) {
  if (value === null) return '—'
  return unit === 'seconds'
    ? seconds.format(value) + ' с'
    : formatValue(value, unit)
}

function itemReason(code: string | null) {
  return code && Object.hasOwn(exclusionReasons, code)
    ? exclusionReasons[code]
    : metricReason(code)
}

export function CohortTable({
  api,
  items,
  unit,
  asOf,
  publishedAt,
}: {
  api: ApiClient
  items: CohortItem[]
  unit: string
  asOf: string
  publishedAt: string | null
}) {
  return (
    <div
      className="cp-cohort__table-scroll"
      tabIndex={0}
      role="region"
      aria-label="Таблица объектов точки"
    >
      <table className="cp-cohort__table">
        <thead>
          <tr>
            <th scope="col">Объект</th>
            <th scope="col">Роль</th>
            <th scope="col">Вклад</th>
            <th scope="col">Причина</th>
            <th scope="col">Основания</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) =>
            item.deleted ? (
              <tr key={'deleted-' + String(index)}>
                <td colSpan={5}>Основание удалено</td>
              </tr>
            ) : (
              <CohortRow
                key={item.object.id + '-' + String(item.itemNo)}
                api={api}
                item={item}
                unit={unit}
                asOf={asOf}
                publishedAt={publishedAt}
              />
            ),
          )}
        </tbody>
      </table>
    </div>
  )
}

function CohortRow({
  api,
  item,
  unit,
  asOf,
  publishedAt,
}: {
  api: ApiClient
  item: Extract<CohortItem, { deleted: false }>
  unit: string
  asOf: string
  publishedAt: string | null
}) {
  const [open, setOpen] = useState(false)
  const { object, role, value, reason, interval } = item
  const isTask = object.kind === 'task'
  const objectName = [object.display_key, object.title].filter(Boolean)
  const label = objectName.length ? objectName.join(' · ') : 'Объект'
  const detailId = useId()
  return (
    <Fragment>
      <tr>
        <td>
          <SourceLink url={object.source_url}>{label}</SourceLink>
        </td>
        <td>{roleLabels[role]}</td>
        <td>{contribution(value, unit)}</td>
        <td>{itemReason(reason) ?? '—'}</td>
        <td>
          {(interval !== null || isTask) && (
            <Button
              size="xl"
              aria-label={
                (open ? 'Свернуть' : 'Раскрыть') + ' основания: ' + label
              }
              aria-expanded={open}
              aria-controls={open ? detailId : undefined}
              onClick={() => {
                setOpen(!open)
              }}
            >
              {open ? 'Свернуть' : 'Раскрыть'}
            </Button>
          )}
        </td>
      </tr>
      {open && (
        <tr>
          <td colSpan={5} id={detailId}>
            {interval && (
              <>
                <h4>Интервал вклада</h4>
                <ContributionInterval interval={interval} />
              </>
            )}
            {isTask && (
              <CohortEvidence
                api={api}
                taskId={object.id}
                asOf={asOf}
                publishedAt={publishedAt}
              />
            )}
          </td>
        </tr>
      )}
    </Fragment>
  )
}
