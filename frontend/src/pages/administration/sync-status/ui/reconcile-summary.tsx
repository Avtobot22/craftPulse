import { Text } from '@gravity-ui/uikit'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel } from '@/shared/config'
import { StatusTime } from '@/shared/ui'

type Reconcile = NonNullable<
  components['schemas']['SyncStatus']['last_reconcile']
>

const outcomes = {
  running: 'Сверка выполняется',
  complete: 'Сверка завершена',
  incomplete: 'Пробел в истории: сверка выявила расхождения',
  failed: 'Пробел в истории: сверка завершилась ошибкой',
} satisfies Record<Reconcile['outcome'], string>

export function ReconcileSummary({
  reconcile,
}: {
  reconcile: OpenCodes<Reconcile> | null
}) {
  return (
    <section>
      <Text as="h2" variant="subheader-2">
        Последняя сверка
      </Text>
      {reconcile === null ? (
        <p>Сверка ещё не проводилась</p>
      ) : (
        <dl className="cp-sync-status__details">
          <dt>Начало</dt>
          <dd>
            <StatusTime value={reconcile.started_at} />
          </dd>
          <dt>Конец</dt>
          <dd>
            <StatusTime value={reconcile.finished_at} />
          </dd>
          <dt>Исход</dt>
          <dd>{contractLabel(outcomes, reconcile.outcome)}</dd>
          <dt>Ожидаемые объекты</dt>
          <dd>{reconcile.expected_count ?? 'Нет данных'}</dd>
          <dt>Найдено в сверке (объектов загружено)</dt>
          <dd>{reconcile.observed_count ?? 'Нет данных'}</dd>
          <dt>Отсутствующие объекты</dt>
          <dd>{reconcile.missing_count ?? 'Нет данных'}</dd>
          <dt>Изменённые объекты</dt>
          <dd>{reconcile.changed_count ?? 'Нет данных'}</dd>
        </dl>
      )}
    </section>
  )
}
