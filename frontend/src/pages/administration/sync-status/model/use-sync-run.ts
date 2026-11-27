import {
  skipToken,
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import {
  getSyncOperation,
  requestSyncRun,
  type ApiClient,
  type components,
  type SyncOperation,
} from '@/shared/api'

export function useSyncRun(
  api: ApiClient,
  connectionId: string,
  refresh: () => Promise<void>,
) {
  const client = useQueryClient()
  const key = ['connections', 'last-requested-run', connectionId]
  const mutationKey = ['connections', 'request-sync-run', connectionId]
  const mutating = useIsMutating({ mutationKey, exact: true }) > 0
  const { data: initial } = useQuery<SyncOperation>({
    queryKey: key,
    queryFn: skipToken,
    // The operation is available through the API for 24 hours after completion.
    gcTime: 24 * 60 * 60 * 1_000,
  })
  const operation = useQuery(getSyncOperation(api, initial))
  const lock = useRef(false)
  const handled = useRef<string>(undefined)

  const request = useMutation({
    mutationKey,
    mutationFn: (body: components['schemas']['SyncRunRequest']) =>
      requestSyncRun(api, connectionId, body, AbortSignal.timeout(30_000)),
    onSuccess: async (result) => {
      client.setQueryData(key, result)
      await refresh()
    },
    onError: refresh,
    onSettled: () => {
      lock.current = false
    },
  })

  const result = operation.data
  const active = result?.status === 'queued' || result?.status === 'running'
  const terminalId = result && !active ? result.id : undefined
  useEffect(() => {
    if (!terminalId || handled.current === terminalId) return
    handled.current = terminalId
    void refresh()
  }, [refresh, terminalId])

  const busy = mutating || active
  function start(body: components['schemas']['SyncRunRequest']) {
    if (busy || lock.current || client.isMutating({ mutationKey, exact: true }))
      return
    lock.current = true
    request.mutate(body)
  }

  return { start, busy, result, operation, requestError: request.error }
}
