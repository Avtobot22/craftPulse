import { Link } from 'react-router'
import { ReceiptStatusBadge } from '@/entities/receipt'
import type { components, OpenCodes } from '@/shared/api'
import { adminLabels, adminLinks, contractLabel } from '@/shared/config'
import { DataTable, StatusTime } from '@/shared/ui'

type ReceiptSummary = OpenCodes<components['schemas']['ReceiptSummary']>

export function ReceiptJournalTable({
  receipts,
}: {
  receipts: readonly ReceiptSummary[]
}) {
  return (
    <DataTable label="Пакеты в журнале приёма">
      <thead>
        <tr>
          <th scope="col">Квитанция</th>
          <th scope="col" aria-sort="descending">
            Время приёма
          </th>
          <th scope="col">Канал</th>
          <th scope="col">Статус</th>
          <th scope="col">Схема</th>
          <th scope="col">Записей</th>
          <th scope="col">Принято</th>
          <th scope="col">Отклонено</th>
        </tr>
      </thead>
      <tbody>
        {receipts.map((receipt) => (
          <tr key={receipt.id}>
            <th scope="row">
              <Link to={adminLinks.receipt(receipt.id)}>{receipt.id}</Link>
            </th>
            <td>
              <StatusTime value={receipt.received_at} />
            </td>
            <td>
              {contractLabel(adminLabels.receiptChannel, receipt.channel)}
            </td>
            <td>
              <ReceiptStatusBadge status={receipt.status} />
            </td>
            <td>
              {[receipt.schema_id, receipt.schema_version]
                .filter(Boolean)
                .join(' · ') || 'Не указана'}
            </td>
            <td>{receipt.record_count}</td>
            <td>{receipt.accepted_count}</td>
            <td>{receipt.rejected_count}</td>
          </tr>
        ))}
      </tbody>
    </DataTable>
  )
}
