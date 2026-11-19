import { Provider } from '@gravity-ui/uikit'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { AppRuntime } from './runtime'

export function AppProviders({
  runtime,
  children,
}: {
  runtime: AppRuntime
  children: ReactNode
}) {
  return (
    <Provider theme="light" lang="ru">
      <QueryClientProvider client={runtime.queryClient}>
        {children}
      </QueryClientProvider>
    </Provider>
  )
}
