import { QueryCache, QueryClient } from '@tanstack/react-query'
import { ApiClient, isUnavailableError } from '@/shared/api'
import { SessionController } from '@/shared/auth'

export function createAppRuntime(
  fetcher: typeof fetch = globalThis.fetch.bind(globalThis),
) {
  const queryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (error, query) => {
        if (isUnavailableError(error)) {
          // Preserve the denial for observers while discarding any previous success.
          query.setState({ data: undefined, dataUpdatedAt: 0 })
        }
      },
    }),
    defaultOptions: { queries: { retry: false } },
  })
  const api = new ApiClient(fetcher)
  const session = new SessionController(api, queryClient)
  return { api, queryClient, session }
}
export type AppRuntime = ReturnType<typeof createAppRuntime>
