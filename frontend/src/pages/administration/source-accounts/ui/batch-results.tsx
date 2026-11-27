import { Text } from '@gravity-ui/uikit'
import { useLayoutEffect, useRef } from 'react'
import type { components } from '@/shared/api'
import { ErrorState } from '@/shared/ui'
import type { RowResult } from '../model/assignments'
import { accountLabel, outcomeLabel } from '../model/assignments'
import { accountRowId } from './accounts-table'

export function BatchResults({
  receipt,
  results,
  refreshing,
  needsRefresh,
  onRetry,
}: {
  receipt: components['schemas']['AccountAssignmentBatchResult'] | undefined
  results: ReadonlyMap<string, RowResult>
  refreshing: boolean
  needsRefresh: boolean
  onRetry: () => void
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  useLayoutEffect(() => {
    if (receipt) heading.current?.focus()
  }, [receipt])

  if (!receipt) return null

  return (
    <section aria-labelledby="account-results-heading">
      <Text
        as="h2"
        variant="subheader-2"
        id="account-results-heading"
        ref={heading}
        tabIndex={-1}
      >
        Результат подтверждения
      </Text>
      <ul className="cp-accounts__outcomes">
        {receipt.items.map(({ source_account_id }) => {
          const result = results.get(source_account_id)
          if (!result) return null
          const rowId = accountRowId(source_account_id)
          return (
            <li key={rowId}>
              <a
                href={`#${rowId}`}
                onClick={(event) => {
                  event.preventDefault()
                  document.getElementById(rowId)?.focus()
                }}
              >
                {accountLabel(result.account)}
              </a>
              {' · '}
              {outcomeLabel(result)}
              {result.refresh === 'missing' && ' · Учётная запись недоступна'}
            </li>
          )
        })}
      </ul>
      {refreshing && <p role="status">Загружаем актуальные привязки…</p>}
      {!refreshing && needsRefresh && (
        <ErrorState
          reason="Не удалось перечитать затронутые привязки"
          onRetry={onRetry}
        />
      )}
    </section>
  )
}
