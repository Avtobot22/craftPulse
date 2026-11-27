import { Button, Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useOutletContext, useParams } from 'react-router'
import { ApiError, getReceipt, isUnavailableError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { readTechnicalId } from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'
import { ReceiptDetails } from './receipt-details'
import { ReceiptRecords } from './receipt-records'
import './receipt-page.css'

export function ReceiptPage({ api }: { api: ApiClient }) {
  const { actions } = useOutletContext<CurrentUser>()
  const id = readTechnicalId(useParams().receiptId)
  const allowed = actions.includes('receipts.read')
  const query = useQuery({
    ...getReceipt(api, id ?? ''),
    enabled: Boolean(id && allowed),
  })

  if (!allowed) return <UnavailableRedirect forbidden />
  if (!id || isUnavailableError(query.error))
    return <UnavailableRedirect error={query.error} />

  const error = query.error instanceof ApiError ? query.error : null
  const correlationId = error?.problem?.correlation_id
  const receipt = error?.kind === 'auth-required' ? undefined : query.data

  return (
    <section className="cp-receipt" aria-labelledby="receipt-heading">
      <Flex
        alignItems="center"
        justifyContent="space-between"
        gap="spacing-3"
        wrap="wrap"
      >
        <Text as="h1" variant="header-1" id="receipt-heading">
          Квитанция приёма
        </Text>
        {receipt && !query.isError && (
          <Button
            size="xl"
            loading={query.isFetching}
            onClick={() => void query.refetch()}
          >
            Обновить
          </Button>
        )}
      </Flex>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось обновить квитанцию"
          {...(correlationId ? { correlationId } : {})}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {receipt && (
        <>
          <ReceiptDetails receipt={receipt} actions={actions} />
          {receipt.status !== 'quarantined' &&
            receipt.status !== 'duplicate' && (
              <ReceiptRecords key={id} api={api} receiptId={id} />
            )}
        </>
      )}
    </section>
  )
}
