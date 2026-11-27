import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef } from 'react'
import { readConnection } from '@/shared/api'
import type { ApiClient, components } from '@/shared/api'
import type { Credentials } from '../model/connection-draft'
import { writeFailure } from '../model/connection-issues'

type Schema = components['schemas']
export type ConnectionWrite = {
  fields: ReadonlySet<string>
} & (
  | { kind: 'create'; body: Omit<Schema['ConnectionCreate'], 'credentials'> }
  | {
      kind: 'update'
      id: string
      etag: string
      body: Omit<Schema['ConnectionPatch'], 'credentials'>
    }
)

export function useSaveConnection(api: ApiClient) {
  const cache = useQueryClient()
  const credentials = useRef<Credentials | undefined>(undefined)
  const { mutateAsync, isPending, reset } = useMutation({
    gcTime: 0,
    retry: false,
    networkMode: 'always',
    mutationFn: async (write: ConnectionWrite) => {
      const signal = AbortSignal.timeout(30_000)
      try {
        const response =
          write.kind === 'create'
            ? await api.createConnection(
                { ...write.body, credentials: credentials.current ?? {} },
                readConnection,
                signal,
              )
            : await api.updateConnection(
                write.id,
                {
                  ...write.body,
                  ...(credentials.current
                    ? { credentials: credentials.current }
                    : {}),
                },
                readConnection,
                write.etag,
                signal,
              )
        return { kind: 'saved', response } as const
      } catch (error) {
        return writeFailure(error, write.fields)
      }
    },
  })

  async function save(write: ConnectionWrite, values: Credentials | undefined) {
    credentials.current = values
    const mutationCache = cache.getMutationCache()
    try {
      return await mutateAsync(write)
    } finally {
      credentials.current = undefined
      reset()
      // reset detaches the observer. Remove the settled entry immediately.
      const entry = mutationCache
        .getAll()
        .find((mutation) => mutation.state.variables === write)
      if (entry) mutationCache.remove(entry)
    }
  }

  return { save, isPending }
}
