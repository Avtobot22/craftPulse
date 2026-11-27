import { Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Link, useOutletContext, useParams } from 'react-router'
import { getReceipt, isUnavailableError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { adminLabels, adminLinks } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'

export function ReceiptPage({ api }: { api: ApiClient }) {
  const { actions } = useOutletContext<CurrentUser>()
  const { receiptId = '' } = useParams()
  const receipt = useQuery(getReceipt(api, receiptId))
  const data = isUnavailableError(receipt.error) ? undefined : receipt.data
  const labels: Readonly<Record<string, string>> = adminLabels.receiptStatus
  return (
    <Flex as="section" direction="column" gap="spacing-4">
      <Text as="h1" variant="header-1">
        Квитанция приёма
      </Text>
      {receipt.isPending && <LoadingState />}
      {receipt.error && (
        <ErrorState
          reason="Не удалось загрузить квитанцию"
          retrying={receipt.isFetching}
          onRetry={() => {
            void receipt.refetch()
          }}
        />
      )}
      {data && (
        <>
          <p role="status">
            {Object.hasOwn(labels, data.status)
              ? labels[data.status]
              : 'Статус неизвестен'}
          </p>
          {actions.includes('imports.create') && (
            <Link
              to={adminLinks.importPackage({
                ...(data.schema_id ? { schema_id: data.schema_id } : {}),
                ...(data.schema_version
                  ? { schema_version: data.schema_version }
                  : {}),
              })}
              state={{ returnReceiptId: receiptId }}
            >
              Исправить и повторить
            </Link>
          )}
        </>
      )}
      {actions.includes('sync.read') && (
        <Link to={adminLinks.ingestConflicts()}>
          Конфликты приёма организации
        </Link>
      )}
    </Flex>
  )
}
