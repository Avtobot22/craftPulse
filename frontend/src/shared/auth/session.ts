import { queryOptions } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import type { ApiClient } from '@/shared/api'
import { ApiError } from '@/shared/api'
import { readCurrentUser } from './current-user'

export type SessionState =
  'checking' | 'authenticated' | 'expired' | 'signed-out'

export class SessionController {
  private state: SessionState = 'checking'
  private listeners = new Set<() => void>()
  private unsubscribe: () => void

  private readonly api: ApiClient
  private readonly queries: QueryClient

  constructor(api: ApiClient, queries: QueryClient) {
    this.api = api
    this.queries = queries
    this.unsubscribe = api.onAuthRequired(() => {
      this.expire()
    })
  }

  getSnapshot = (): SessionState => this.state
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  currentUserOptions() {
    return queryOptions({
      queryKey: ['session', 'current-user'],
      staleTime: 0,
      refetchOnWindowFocus: 'always',
      retry: false,
      queryFn: async ({ signal }) => {
        const response = await this.api.read('/me', readCurrentUser, { signal })
        if (this.state === 'expired' || this.state === 'signed-out')
          throw new ApiError('session-ended')
        this.setState('authenticated')
        return response.data
      },
    })
  }

  async logout(): Promise<void> {
    await this.api.logout()
    this.api.endSession()
    this.queries.clear()
    this.setState('signed-out')
  }

  private expire(): void {
    this.api.endSession()
    this.queries.clear()
    this.setState('expired')
  }

  private setState(state: SessionState): void {
    if (this.state === state) return
    this.state = state
    for (const listener of this.listeners) listener()
  }

  dispose(): void {
    this.unsubscribe()
    this.listeners.clear()
  }
}
