import { Flex, Text } from '@gravity-ui/uikit'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useLayoutEffect, useRef } from 'react'
import { useLocation, useNavigate, useOutletContext } from 'react-router'
import type { To } from 'react-router'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { SourceLinkAccess } from './source-link-access'
import { routePaths } from '@/shared/config'
import { ErrorState } from '@/shared/ui'

interface UnavailableReturn {
  to: To
  focusId?: string
  viaHistory?: boolean
}

export function UnavailablePage({
  api,
  currentUser,
  correlationId,
  forbidden,
}: {
  api: ApiClient
  currentUser?: CurrentUser
  correlationId?: string | undefined
  forbidden?: boolean
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const state: unknown = location.state
  const navigation = state && typeof state === 'object' ? state : {}
  const context = useOutletContext<{
    user?: CurrentUser['user']
    organization?: CurrentUser['organization']
    unavailableReturn?: UnavailableReturn
  } | null>()
  const heading = useRef<HTMLHeadingElement>(null)
  const queries = useQueryClient()

  useLayoutEffect(() => {
    heading.current?.focus()
  }, [location.key])

  useEffect(() => {
    // Retire detached detail queries and lists that could contain the denied object.
    queries.removeQueries({
      type: 'inactive',
      predicate: ({ queryKey }) => queryKey[0] !== 'session',
    })
  }, [location.key, queries])

  const code =
    correlationId ??
    ('correlationId' in navigation &&
    typeof navigation.correlationId === 'string' &&
    /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu.test(
      navigation.correlationId,
    )
      ? navigation.correlationId
      : undefined)
  const denied =
    forbidden ?? ('forbidden' in navigation && navigation.forbidden === true)
  const back = context?.unavailableReturn
  const user = currentUser ?? context
  const identity =
    user?.organization && user.user
      ? `${user.organization.id}.${user.user.id}`
      : undefined
  return (
    <Flex direction="column" gap="spacing-4">
      <ErrorState
        title={
          <Text as="h1" variant="header-1" ref={heading} tabIndex={-1}>
            Объект или область недоступны
          </Text>
        }
        reason={
          denied
            ? 'Это действие недоступно вашей роли.'
            : 'Не удалось открыть объект или область.'
        }
        {...(code ? { correlationId: code } : {})}
        action={{
          label: 'К списку',
          onClick: () => {
            if (back?.viaHistory) {
              void navigate(-1)
              return
            }
            void navigate(back?.to ?? routePaths.home, {
              replace: true,
              state: back?.focusId
                ? { taskReturnFocus: back.focusId }
                : undefined,
            })
          },
        }}
      />
      <SourceLinkAccess
        key={location.key}
        api={api}
        identity={identity}
        deniedFrom={
          'deniedFrom' in navigation ? navigation.deniedFrom : undefined
        }
      />
    </Flex>
  )
}
