import { Text } from '@gravity-ui/uikit'
import type { components, OpenCodes } from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'
import { ContractReason, DataStateBadge, StatusTime } from '@/shared/ui'
import './sync-data-kind.css'

const hours = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 })

function formatLag(seconds: number | null) {
  if (seconds === null) return 'Нет данных'
  if (seconds === 0) return '0 ч'
  if (seconds < 360) return 'Менее 0,1 ч'
  const tenths = Math.floor(seconds / 360)
  const days = Math.floor(tenths / 240)
  const remainder = hours.format((tenths % 240) / 10)
  return days > 0
    ? `${hours.format(days)} дн. ${remainder} ч`
    : `${remainder} ч`
}

export function SyncDataKind({
  data,
}: {
  data: OpenCodes<components['schemas']['SyncDataKindStatus']>
}) {
  return (
    <section>
      <Text as="h2" variant="subheader-2">
        {contractLabel(adminLabels.dataKind, data.data_kind)}
      </Text>
      <dl className="cp-sync-data-kind">
        <dt>Фаза</dt>
        <dd>{contractLabel(adminLabels.syncPhase, data.phase)}</dd>
        <dt>Последняя попытка</dt>
        <dd>
          <StatusTime value={data.last_attempt_at} />
        </dd>
        <dt>Исход попытки</dt>
        <dd>
          {data.last_outcome === null
            ? 'Нет данных'
            : contractLabel(adminLabels.syncOutcome, data.last_outcome)}
        </dd>
        <dt>Ошибка</dt>
        <dd>
          {data.last_error_code === null ? (
            data.last_outcome === 'error' ? (
              'Причина не указана'
            ) : (
              'Нет данных'
            )
          ) : (
            <ContractReason
              code={data.last_error_code}
              labels={adminLabels.syncError}
            />
          )}
        </dd>
        <dt>Страница с ошибкой</dt>
        <dd>{data.failed_page_present ? 'Есть' : 'Нет'}</dd>
        <dt>Последний успешный сбор</dt>
        <dd>
          <StatusTime value={data.last_success_at} />
        </dd>
        <dt>Данные сверены до</dt>
        <dd>
          <StatusTime value={data.watermark} />
        </dd>
        <dt>Отставание</dt>
        <dd>{formatLag(data.lag_seconds)}</dd>
        <dt>История с</dt>
        <dd>
          <StatusTime value={data.history_from} />
        </dd>
        <dt>Полнота</dt>
        <dd>
          {data.completeness === 'partial' ? (
            <DataStateBadge state="PROVISIONAL" />
          ) : data.completeness === 'unknown' ? (
            <DataStateBadge state="UNKNOWN" />
          ) : (
            contractLabel(adminLabels.syncCompleteness, data.completeness)
          )}
        </dd>
        {data.unknown_fields.length > 0 && (
          <>
            <dt>Поля с неизвестными значениями</dt>
            <dd>{data.unknown_fields.join(', ')}</dd>
          </>
        )}
        {data.affected_metrics.length > 0 && (
          <>
            <dt>Затронутые показатели</dt>
            <dd>{data.affected_metrics.join(', ')}</dd>
          </>
        )}
      </dl>
    </section>
  )
}
