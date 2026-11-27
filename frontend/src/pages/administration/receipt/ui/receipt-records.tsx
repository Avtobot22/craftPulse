import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import { isUnavailableError, listReceiptRecords } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'
import {
  CursorPagination,
  DataTable,
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
  useCursorSearch,
} from '@/shared/ui'

const outcomes = [
  'accepted',
  'replay',
  'conflict',
  'rejected',
  'suppressed',
] as const satisfies readonly (keyof typeof adminLabels.recordOutcome)[]

export function ReceiptRecords({
  api,
  receiptId,
}: {
  api: ApiClient
  receiptId: string
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  const { search, filter, page } = useCursorSearch()
  const outcome = outcomes.find((code) => code === search.get('outcome'))
  const records = useQuery(
    listReceiptRecords(api, receiptId, {
      limit: page.limit,
      ...(page.cursor ? { cursor: page.cursor } : {}),
      ...(outcome ? { outcome } : {}),
    }),
  )

  if (isUnavailableError(records.error))
    return <UnavailableRedirect error={records.error} />

  const data = records.isError ? undefined : records.data
  return (
    <section
      className="cp-receipt__records"
      aria-labelledby="receipt-records-heading"
    >
      <Text
        as="h2"
        variant="header-2"
        id="receipt-records-heading"
        ref={heading}
        tabIndex={-1}
      >
        Записи пакета
      </Text>
      <label className="cp-receipt__filter">
        Исход записи
        <select
          value={outcome ?? ''}
          onChange={(event) => {
            filter('outcome', event.target.value)
          }}
        >
          <option value="">Все исходы</option>
          {outcomes.map((code) => (
            <option key={code} value={code}>
              {adminLabels.recordOutcome[code]}
            </option>
          ))}
        </select>
      </label>
      {records.isPending && <LoadingState />}
      {records.isError && (
        <ErrorState
          reason="Не удалось загрузить записи пакета"
          retrying={records.isFetching}
          onRetry={() => void records.refetch()}
        />
      )}
      {data && (
        <>
          {data.data.length ? (
            <DataTable label="Исходы записей">
              <thead>
                <tr>
                  <th scope="col">Номер</th>
                  <th scope="col">Вид</th>
                  <th scope="col">Исходные координаты</th>
                  <th scope="col">Исход</th>
                  <th scope="col">Причина</th>
                </tr>
              </thead>
              <tbody>
                {data.data.map((record) => (
                  <tr key={record.record_no}>
                    <th scope="row">{record.record_no}</th>
                    <td>{record.record_kind}</td>
                    <td>{record.native_key ?? 'идентичность не определена'}</td>
                    <td>
                      {contractLabel(adminLabels.recordOutcome, record.outcome)}
                    </td>
                    <td>
                      {record.reason_code ? (
                        <>
                          {Object.hasOwn(
                            adminLabels.recordReason,
                            record.reason_code,
                          ) && (
                            <div>
                              {contractLabel(
                                adminLabels.recordReason,
                                record.reason_code,
                              )}
                            </div>
                          )}
                          <code>{record.reason_code}</code>
                        </>
                      ) : (
                        '–'
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </DataTable>
          ) : (
            <EmptyState
              reason={outcome || page.cursor ? 'filters' : 'source'}
              title={
                outcome || page.cursor
                  ? 'Записей по фильтру на этой странице нет'
                  : 'В пакете нет записей'
              }
            />
          )}
          <CursorPagination
            {...page}
            label="Страницы записей"
            next={data.next_cursor}
            received={data.data.length}
            goTo={(...args) => {
              heading.current?.focus()
              page.goTo(...args)
            }}
          />
        </>
      )}
    </section>
  )
}
