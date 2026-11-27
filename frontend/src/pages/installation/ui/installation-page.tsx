import { Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useOutletContext } from 'react-router'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import { getLatestInstallationProtocol } from '../api/installation-protocol'
import { ProtocolDetails } from './protocol-details'

export function InstallationPage({ api }: { api: ApiClient }) {
  const { organization } = useOutletContext<CurrentUser>()
  const query = useQuery(getLatestInstallationProtocol(api, organization.id))
  const error = query.error instanceof ApiError ? query.error : null
  if (error?.kind === 'forbidden') return <UnavailableRedirect error={error} />

  return (
    <Flex as="section" direction="column" gap="spacing-4">
      <Text as="h1" variant="header-1">
        Диагностика установки
      </Text>
      {query.isPending ? (
        <LoadingState />
      ) : error?.kind === 'not-found' ? (
        <EmptyState
          reason="history"
          title="Проверка установки ещё не запускалась"
          description="Запустите проверку вне сайта по инструкции установки."
        />
      ) : query.isError ? (
        <ErrorState
          reason="Не удалось загрузить протокол установки"
          {...(error?.problem
            ? { correlationId: error.problem.correlation_id }
            : {})}
          retrying={query.isFetching}
          onRetry={() => {
            void query.refetch()
          }}
        />
      ) : (
        <ProtocolDetails protocol={query.data} />
      )}
    </Flex>
  )
}
