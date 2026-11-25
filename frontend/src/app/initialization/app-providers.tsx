import { Provider, Toaster, ToasterProvider } from '@gravity-ui/uikit'
import { QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import type { ReactNode } from 'react'
import { layoutTheme } from '@/shared/config'
import type { AppRuntime } from './runtime'
import { uikitDefaults } from './uikit-defaults'

export function AppProviders({
  runtime,
  children,
}: {
  runtime: AppRuntime
  children: ReactNode
}) {
  const [toaster] = useState(() => new Toaster())
  return (
    <Provider
      theme="light"
      lang="ru"
      defaultProps={uikitDefaults}
      layout={{ config: layoutTheme }}
    >
      <ToasterProvider toaster={toaster}>
        <QueryClientProvider client={runtime.queryClient}>
          {children}
        </QueryClientProvider>
      </ToasterProvider>
    </Provider>
  )
}
