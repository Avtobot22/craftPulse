import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Navigate, useParams } from 'react-router'
import { getConnection, isUnavailableError, listAdapters } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { readTechnicalId, routePaths } from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'
import { isBuiltinAdapter } from '../model/connection-draft'
import { ConnectionEditor } from './connection-editor'
import './connection-form.css'

export function ConnectionFormPage({ api }: { api: ApiClient }) {
  const { connectionId } = useParams()
  const id = readTechnicalId(connectionId)
  if (connectionId && !id)
    return <Navigate to={routePaths.unavailable} replace />
  return <ConnectionForm key={id ?? 'new'} api={api} id={id} />
}

function ConnectionForm({
  api,
  id,
}: {
  api: ApiClient
  id: string | undefined
}) {
  const adapters = useQuery(listAdapters(api))
  const connection = useQuery({
    ...getConnection(api, id ?? ''),
    enabled: Boolean(id),
    refetchOnMount: 'always',
  })
  // Initialize the editor with the current status and ETag, even on a cached route.
  const loading =
    adapters.isPending || Boolean(id && !connection.isFetchedAfterMount)
  const error =
    (adapters.data ? null : adapters.error) ??
    (id && !connection.data ? connection.error : null)
  const initial = id ? connection.data : undefined

  if (isUnavailableError(error)) return <UnavailableRedirect error={error} />

  return (
    <section className="cp-connection">
      <Text as="h1" variant="header-1">
        {id ? 'Настройки подключения' : 'Новое подключение'}
      </Text>
      {error ? (
        <ErrorState
          reason="Не удалось загрузить настройки подключения"
          onRetry={() => {
            void adapters.refetch()
            if (id) void connection.refetch()
          }}
          retrying={adapters.isFetching || Boolean(id && connection.isFetching)}
        />
      ) : loading ? (
        <LoadingState />
      ) : (
        adapters.data && (
          <ConnectionEditor
            api={api}
            adapters={adapters.data.data.filter(isBuiltinAdapter)}
            initial={initial}
          />
        )
      )}
    </section>
  )
}
