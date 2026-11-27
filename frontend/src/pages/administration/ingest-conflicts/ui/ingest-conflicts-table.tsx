import { generatePath, Link } from 'react-router'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { StatusTime } from '@/shared/ui'
import type { IngestConflict } from '../model/conflict-resolution'
import { ConflictResolution } from './conflict-resolution'

function ReceiptLink({ id, recordNo }: { id: string; recordNo: number }) {
  return (
    <>
      <Link
        to={generatePath(routePaths.receipt, {
          receiptId: encodeURIComponent(id),
        })}
      >
        {id}
      </Link>
      <div>Запись № {recordNo}</div>
    </>
  )
}

export function IngestConflictsTable({
  api,
  conflicts,
  canResolve,
}: {
  api: ApiClient
  conflicts: readonly IngestConflict[]
  canResolve: boolean
}) {
  return (
    <div
      className="cp-ingest-conflicts__scroll"
      role="region"
      aria-label="Таблица конфликтов приёма"
      tabIndex={0}
    >
      <table aria-label="Конфликты приёма">
        <thead>
          <tr>
            <th scope="col">Исходные координаты</th>
            <th scope="col">Обнаружен</th>
            <th scope="col">Новая запись</th>
            <th scope="col">Принятая запись</th>
            <th scope="col">Статус</th>
            <th scope="col">Решение</th>
          </tr>
        </thead>
        <tbody>
          {conflicts.map((conflict) => (
            <ConflictRow
              key={conflict.id}
              api={api}
              conflict={conflict}
              canResolve={canResolve}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ConflictRow({
  api,
  conflict,
  canResolve,
}: {
  api: ApiClient
  conflict: IngestConflict
  canResolve: boolean
}) {
  return (
    <tr>
      <th scope="row">
        <code>{conflict.native_key}</code>
      </th>
      <td>
        <StatusTime value={conflict.detected_at} />
      </td>
      <td>
        <ReceiptLink id={conflict.receipt_id} recordNo={conflict.record_no} />
      </td>
      <td>
        <ReceiptLink
          id={conflict.existing_receipt_id}
          recordNo={conflict.existing_record_no}
        />
      </td>
      <ConflictResolution
        api={api}
        conflict={conflict}
        canResolve={canResolve}
      />
    </tr>
  )
}
