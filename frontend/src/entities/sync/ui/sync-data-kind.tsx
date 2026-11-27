import { Text } from '@gravity-ui/uikit'
import type { components, OpenCodes } from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'
import {
  ContractReason,
  DataStateBadge,
  StatusTime,
  TechnicalValue,
} from '@/shared/ui'
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
  summary = false,
}: {
  data: OpenCodes<components['schemas']['SyncDataKindStatus']>
  summary?: boolean
}) {
  return (
    <section>
      <Text as="h2" variant={summary ? 'subheader-1' : 'subheader-2'}>
        {contractLabel(adminLabels.dataKind, data.data_kind)}
      </Text>
      <dl className="cp-sync-data-kind">
        <dt>Фаза</dt>
        <dd>{contractLabel(adminLabels.syncPhase, data.phase)}</dd>
        {!summary && (
          <>
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
          </>
        )}
        <dt>Ошибка</dt>
        <dd>
          {data.last_error_code === null ? (
            data.last_outcome === 'error' ? (
              'Причина не указана'
            ) : data.last_outcome === 'success' ? (
              'Нет ошибок'
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
        <dd>
          {data.failed_page_present
            ? 'Сбор остановился на странице, повтор продолжит с неё'
            : 'Нет'}
        </dd>
        <dt>Последний успешный сбор</dt>
        <dd>
          <StatusTime value={data.last_success_at} />
        </dd>
        {!summary && (
          <>
            <dt>Выборка подтверждена сверкой до</dt>
            <dd>
              <StatusTime value={data.watermark} />
            </dd>
          </>
        )}
        <dt>Отставание</dt>
        <dd>{formatLag(data.lag_seconds)}</dd>
        {!summary && (
          <>
            <dt>История известна с</dt>
            <dd>
              <StatusTime value={data.history_from} />
            </dd>
          </>
        )}
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
        {!summary && data.unknown_fields.length > 0 && (
          <>
            <dt>Поля с неизвестными значениями</dt>
            <dd>
              <ul className="cp-sync-data-kind__fields">
                {data.unknown_fields.map((field) => (
                  <li key={field}>
                    <TechnicalValue label="Код поля" value={field} />
                  </li>
                ))}
              </ul>
            </dd>
          </>
        )}
        {!summary && data.affected_metrics.length > 0 && (
          <>
            <dt>Затронутые показатели</dt>
            <dd>{data.affected_metrics.join(', ')}</dd>
          </>
        )}
      </dl>
    </section>
  )
}
