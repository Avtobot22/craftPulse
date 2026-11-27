import { Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import { Link, useOutletContext } from 'react-router'
import { ApiError, isUnavailableError, listReceipts } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { adminLinks } from '@/shared/config'
import {
  CursorPagination,
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
  useCursorSearch,
} from '@/shared/ui'
import {
  readJournalFilters,
  ReceiptJournalFilters,
} from './receipt-journal-filters'
import { ReceiptJournalTable } from './receipt-journal-table'
import './receipt-journal.css'

export function ReceiptJournalPage({ api }: { api: ApiClient }) {
  const { actions, organization, user } = useOutletContext<CurrentUser>()
  const identity = organization.id + ':' + user.id
  const allowed = actions.includes('sync.read')
  const heading = useRef<HTMLHeadingElement>(null)
  const { search, filter, page } = useCursorSearch()
  const filters = readJournalFilters(search)
  const receipts = useQuery({
    ...listReceipts(api, {
      ...filters,
      limit: page.limit,
      ...(page.cursor ? { cursor: page.cursor } : {}),
    }),
    enabled: allowed,
  })

  if (!allowed) return <UnavailableRedirect forbidden />
  if (isUnavailableError(receipts.error))
    return <UnavailableRedirect error={receipts.error} />

  const data = receipts.isError ? undefined : receipts.data
  const error = receipts.error instanceof ApiError ? receipts.error : null
  const correlationId = error?.problem?.correlation_id
  const filtered = Object.keys(filters).length > 0 || Boolean(page.cursor)

  return (
    <section
      className="cp-receipt-journal"
      aria-labelledby="receipt-journal-heading"
    >
      <Flex
        alignItems="center"
        justifyContent="space-between"
        gap="spacing-3"
        wrap="wrap"
      >
        <Text
          as="h1"
          id="receipt-journal-heading"
          variant="header-1"
          ref={heading}
          tabIndex={-1}
        >
          Журнал приёма
        </Text>
        <Link to={adminLinks.ingestConflicts()}>
          Конфликты приёма организации
        </Link>
      </Flex>
      <ReceiptJournalFilters
        api={api}
        actions={actions}
        identity={identity}
        filters={filters}
        onChange={filter}
      />
      {receipts.isPending && <LoadingState />}
      {receipts.isError && (
        <ErrorState
          reason="Не удалось загрузить журнал приёма"
          {...(correlationId ? { correlationId } : {})}
          retrying={receipts.isFetching}
          onRetry={() => void receipts.refetch()}
        />
      )}
      {data && (
        <>
          {data.data.length ? (
            <ReceiptJournalTable receipts={data.data} />
          ) : (
            <EmptyState
              reason={filtered ? 'filters' : 'source'}
              title={
                filtered
                  ? 'Пакетов по фильтру на этой странице нет'
                  : 'Пакеты ещё не поступали'
              }
            />
          )}
          <CursorPagination
            {...page}
            label="Страницы журнала приёма"
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
