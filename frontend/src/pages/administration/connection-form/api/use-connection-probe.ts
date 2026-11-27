import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import {
  activateConnection,
  ApiError,
  getConnection,
  getConnectionProbe,
  isStrongETag,
  startConnectionProbe,
} from '@/shared/api'
import type { ApiClient, ApiResponse } from '@/shared/api'
import type { Connection } from '../model/connection-draft'

export function canActivateConnection(status: string | undefined) {
  return status === 'draft' || status === 'access_lost'
}

export function useConnectionProbe(
  api: ApiClient,
  baseline: ApiResponse<Connection> | undefined,
) {
  const cache = useQueryClient()
  const [probeId, setProbeId] = useState<string>()
  const [stale, setStale] = useState(false)
  const [pending, setPending] = useState<'start' | 'activate'>()
  const [notice, setNotice] = useState<string>()
  const [activationError, setActivationError] = useState<string>()
  const sending = useRef(false)
  const id = baseline?.data.id
  const etag = baseline?.etag
  const options = getConnectionProbe(api, id ?? '', probeId ?? '')
  const query = useQuery({
    ...options,
    enabled: Boolean(id && probeId),
    // The 202 response seeds the query and sets the delay before the first GET.
    refetchOnMount: false,
  })
  const data = query.data
  // The connection resource uses the quoted row version as its ETag.
  const versionChanged = Boolean(
    !stale &&
    data &&
    /^"[0-9]+"$/u.test(etag ?? '') &&
    etag !== `"${String(data.configuration_version)}"`,
  )
  const obsolete = stale || versionChanged
  const inProgress = data?.status === 'pending' || data?.status === 'running'
  const ready = data?.status === 'succeeded' && !obsolete && !query.isError

  function invalidate() {
    setStale(true)
    setNotice(undefined)
  }

  async function requestProbe(connectionId: string) {
    setProbeId(undefined)
    setStale(false)
    const response = await startConnectionProbe(
      api,
      connectionId,
      AbortSignal.timeout(30_000),
    )
    const { queryKey } = getConnectionProbe(api, connectionId, response.id)
    await cache.cancelQueries({ queryKey })
    cache.setQueryData(queryKey, response)
    setProbeId(response.id)
  }

  async function start() {
    if (!id || !isStrongETag(etag) || sending.current || inProgress) return
    sending.current = true
    setPending('start')
    setNotice(undefined)
    try {
      await requestProbe(id)
    } catch (error) {
      setNotice(
        error instanceof ApiError && error.problem?.code === 'state_conflict'
          ? 'Ещё идёт проверка прежних настроек. Повторите проверку позже.'
          : 'Не удалось запустить проверку подключения. Повторите попытку.',
      )
    } finally {
      sending.current = false
      setPending(undefined)
    }
  }

  async function activate({
    onActivated,
    onVersionConflict,
    onStateConflict,
  }: {
    onActivated: (response: ApiResponse<Connection>) => Promise<void>
    onVersionConflict: () => void
    onStateConflict: () => void
  }) {
    if (!ready || !id || !isStrongETag(etag) || sending.current) return
    sending.current = true
    setPending('activate')
    setActivationError(undefined)
    try {
      const response = await activateConnection(
        api,
        id,
        data.id,
        etag,
        AbortSignal.timeout(30_000),
      )
      await onActivated(response)
    } catch (error) {
      if (error instanceof ApiError && error.kind === 'version-conflict') {
        invalidate()
        onVersionConflict()
      } else if (
        error instanceof ApiError &&
        error.problem?.status === 409 &&
        error.problem.code === 'state_conflict'
      ) {
        setProbeId(undefined)
        onStateConflict()
        setNotice(
          baseline.data.status === 'access_lost'
            ? 'Сервер отклонил восстановление. Для него нужна последняя успешная проверка с актуальными учётными данными.'
            : 'Сервер отклонил подтверждение. Для него нужна последняя успешная проверка.',
        )
        try {
          const latest = await cache.query({
            ...getConnection(api, id),
            staleTime: 0,
            retry: false,
          })
          if (
            latest.etag !== etag ||
            latest.data.status !== baseline.data.status
          ) {
            onVersionConflict()
          } else {
            // The contract exposes lookup by ID only. POST can resume a newer
            // in-flight probe or return a new one for the same configuration.
            await requestProbe(id)
          }
        } catch {
          setNotice(
            'Сервер отклонил подтверждение. Не удалось получить новую проверку. Повторите проверку позже.',
          )
        }
      } else {
        setActivationError(
          'Не удалось подтвердить подключение. Повторите попытку.',
        )
      }
    } finally {
      sending.current = false
      setPending(undefined)
    }
  }

  return {
    data,
    pending,
    notice,
    activationError,
    inProgress,
    obsolete,
    versionChanged,
    ready,
    readError: query.isError,
    reading: query.isFetching,
    refresh: () => void query.refetch(),
    invalidate,
    start,
    activate,
    clearActivationError: () => {
      setActivationError(undefined)
    },
  }
}
