import { Flex, Text } from '@gravity-ui/uikit'
import { Link, useLocation } from 'react-router'
import { ReceiptStatusBadge } from '@/entities/receipt'
import type { components, OpenCodes } from '@/shared/api'
import { safeReturnTo } from '@/shared/auth'
import { adminLabels, adminLinks, contractLabel } from '@/shared/config'
import { StatusTime } from '@/shared/ui'

type Receipt = OpenCodes<components['schemas']['Receipt']>

export function ReceiptDetails({
  receipt,
  actions,
}: {
  receipt: Receipt
  actions: readonly string[]
}) {
  const { search } = useLocation()
  const waiting = receipt.status === 'quarantined'
  const duplicate = receipt.status === 'duplicate'
  const result = receipt.result_url ? safeReturnTo(receipt.result_url) : null
  const counts = [
    ['Всего записей', receipt.record_count],
    ['Принято', receipt.accepted_count],
    ['Отклонено', receipt.rejected_count],
    ['Повторов записей', receipt.replay_count],
    ['Конфликтов', receipt.conflict_count],
  ] as const

  return (
    <>
      <div role="status" aria-atomic="true">
        <ReceiptStatusBadge status={receipt.status} />
        {duplicate && (
          <p>Повтор доставки: пакет уже принят, новых записей нет</p>
        )}
        {receipt.status === 'rejected' && (
          <p>Прежний принятый результат не изменён</p>
        )}
      </div>
      {receipt.status === 'rejected' && receipt.reason_code && (
        <p>
          {Object.hasOwn(adminLabels.receiptReason, receipt.reason_code) && (
            <>
              {contractLabel(adminLabels.receiptReason, receipt.reason_code)}{' '}
              ·{' '}
            </>
          )}
          <code>{receipt.reason_code}</code>
        </p>
      )}
      <dl className="cp-receipt__fields">
        <div>
          <dt>Канал</dt>
          <dd>{contractLabel(adminLabels.receiptChannel, receipt.channel)}</dd>
        </div>
        <div>
          <dt>Время приёма</dt>
          <dd>
            <StatusTime value={receipt.received_at} />
          </dd>
        </div>
        <div>
          <dt>Схема</dt>
          <dd>{receipt.schema_id ?? 'Не указана'}</dd>
        </div>
        <div>
          <dt>Версия схемы</dt>
          <dd>{receipt.schema_version ?? 'Не указана'}</dd>
        </div>
      </dl>
      <dl className="cp-receipt__fields cp-receipt__counts">
        {counts.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>
              <Text variant="header-2">{value}</Text>
            </dd>
          </div>
        ))}
      </dl>
      <p>
        <strong>Затронутые показатели</strong>{' '}
        {receipt.affected_metrics.length
          ? receipt.affected_metrics.join(', ')
          : 'Нет данных'}
      </p>
      <Flex gap="spacing-4" wrap="wrap">
        {duplicate && receipt.duplicate_of && (
          <Link to={adminLinks.receipt(receipt.duplicate_of)}>
            Исходная квитанция
          </Link>
        )}
        {result && (result !== '/' || receipt.result_url === '/') && (
          <Link to={result}>К результату</Link>
        )}
        {!waiting && !duplicate && actions.includes('imports.create') && (
          <Link
            to={adminLinks.importPackage({
              ...(receipt.schema_id ? { schema_id: receipt.schema_id } : {}),
              ...(receipt.schema_version
                ? { schema_version: receipt.schema_version }
                : {}),
            })}
            state={{ returnReceiptId: receipt.id, returnReceiptSearch: search }}
          >
            Исправить и повторить
          </Link>
        )}
        {receipt.conflict_count > 0 &&
          actions.includes('connections.manage') && (
            <Link to={adminLinks.ingestConflicts()}>
              Конфликты приёма организации
            </Link>
          )}
      </Flex>
    </>
  )
}
