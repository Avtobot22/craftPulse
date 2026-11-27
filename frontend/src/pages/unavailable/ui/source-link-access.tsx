import { Alert, Flex, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { ApiError, listSourceLinks } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  readSourceLinkResult,
  rememberSourceLinkRetry,
  sourceLinkReturnPath,
  takeSourceLinkRetry,
} from '../model/source-link-return'

function startErrorMessage(error: unknown) {
  const status = error instanceof ApiError ? error.problem?.status : undefined
  if (status === 409) return 'Источник сейчас не принимает разрешения.'
  if (status === 404) return 'Подключение больше недоступно.'
  return 'Не удалось начать разрешение доступа. Попробуйте ещё раз.'
}

export function SourceLinkAccess({
  api,
  identity,
  deniedFrom,
}: {
  api: ApiClient
  identity: string | undefined
  deniedFrom: unknown
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const result = readSourceLinkResult(location.search)
  const returnPath = sourceLinkReturnPath(location)
  const links = useQuery({
    queryKey: ['source-links', location.key, identity],
    queryFn: ({ signal }) => listSourceLinks(api, signal),
    retry: false,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const offers =
    links.data?.filter(
      ({ state }) =>
        state === 'not_linked' || state === 'expired' || state === 'revoked',
    ) ?? []
  const request = useRef<AbortController | null>(null)
  const [pending, setPending] = useState<string>()
  const [failure, setFailure] = useState<{ id: string; message: string }>()

  useEffect(() => () => request.current?.abort(), [])

  async function authorize(connectionId: string) {
    if (request.current) return
    const controller = new AbortController()
    request.current = controller
    setPending(connectionId)
    setFailure(undefined)
    try {
      const { data } = await api.startSourceLink(
        connectionId,
        returnPath,
        controller.signal,
      )
      if (controller.signal.aborted) return
      if (identity && location.pathname === routePaths.unavailable && !result) {
        rememberSourceLinkRetry(identity, deniedFrom)
      }
      window.location.replace(data.authorization_url)
    } catch (error) {
      if (controller.signal.aborted) return
      request.current = null
      setPending(undefined)
      if (error instanceof ApiError && error.kind === 'auth-required') return
      setFailure({ id: connectionId, message: startErrorMessage(error) })
    }
  }

  function retryTransition() {
    const usesStoredRetry =
      location.pathname === routePaths.unavailable ||
      location.pathname === '/source-links'
    const path = usesStoredRetry
      ? identity && takeSourceLinkRetry(identity)
      : returnPath
    void navigate(path ?? routePaths.home, { replace: true })
  }

  if (!result && offers.length === 0) return null

  return (
    <>
      {result === 'linked' && (
        <Alert
          theme="success"
          title={<span role="status">Доступ к источнику разрешён.</span>}
          actions={
            <Alert.Actions>
              <Alert.Action view="outlined" onClick={retryTransition}>
                Повторить переход
              </Alert.Action>
            </Alert.Actions>
          }
        />
      )}
      {result === 'link_failed' && (
        <Alert
          theme="warning"
          title={
            <span role="status">Не удалось разрешить доступ к источнику.</span>
          }
          actions={
            offers.length === 0 && !links.isPending ? (
              <Alert.Actions>
                <Alert.Action
                  view="outlined"
                  loading={links.isFetching}
                  onClick={() => {
                    void links.refetch()
                  }}
                >
                  Проверить подключения
                </Alert.Action>
              </Alert.Actions>
            ) : undefined
          }
        />
      )}
      {offers.map(({ connection_id, display_name }) => (
        <section key={connection_id} aria-label={display_name}>
          <Alert
            theme="info"
            title={display_name}
            message={
              <Flex direction="column" gap="spacing-2">
                <Text>
                  Разрешите craftPulse читать документы от вашего имени.
                </Text>
                {failure?.id === connection_id && (
                  <Text role="alert">{failure.message}</Text>
                )}
                {pending === connection_id && (
                  <Text role="status">Подготавливаем переход к источнику.</Text>
                )}
              </Flex>
            }
            actions={
              <Alert.Actions>
                <Alert.Action
                  view="outlined"
                  loading={pending === connection_id}
                  disabled={pending !== undefined}
                  onClick={() => {
                    void authorize(connection_id)
                  }}
                >
                  {result === 'link_failed'
                    ? 'Повторить разрешение'
                    : 'Разрешить доступ'}
                </Alert.Action>
              </Alert.Actions>
            }
          />
        </section>
      ))}
    </>
  )
}
