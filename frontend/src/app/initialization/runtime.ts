import { QueryClient } from '@tanstack/react-query'
import { ApiClient } from '@/shared/api'
import { SessionController } from '@/shared/auth'

export function createAppRuntime(
  fetcher: typeof fetch = globalThis.fetch.bind(globalThis),
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const api = new ApiClient(fetcher)
  const session = new SessionController(api, queryClient)
  return { api, queryClient, session }
}
export type AppRuntime = ReturnType<typeof createAppRuntime>
