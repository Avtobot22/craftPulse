import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { Link } from 'react-router'
import { isUnavailableError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { SessionController } from '@/shared/auth'
import { routePaths } from '@/shared/config'
import { ErrorState, LoadingState, UnavailableRedirect } from '@/shared/ui'
import { userDetails, userRoles } from '../api/users-api'
import { RoleEditor } from './role-editor'
import { UserAccess } from './user-access'
import { userName, userStatus } from './user-list'

export function UserDetails({
  api,
  session,
  userId,
  ownProfile,
}: {
  api: ApiClient
  session: SessionController
  userId: string
  ownProfile: boolean
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  useEffect(() => heading.current?.focus(), [])
  const client = useQueryClient()
  const profile = useQuery(userDetails(api, userId))
  const roles = useQuery(userRoles(api, userId))
  const unavailable = [roles.error, profile.error].find(isUnavailableError)
  if (unavailable) return <UnavailableRedirect error={unavailable} />

  const user = profile.data
  return (
    <section aria-labelledby="user-details-title" className="cp-users__panel">
      <Link to={routePaths.users}>Закрыть карточку</Link>
      <h2 id="user-details-title" ref={heading} tabIndex={-1}>
        {user ? userName(user) : 'Доступ пользователя'}
      </h2>
      {profile.isPending && <LoadingState />}
      {profile.isError && (
        <ErrorState
          reason="Не удалось перечитать статус пользователя"
          retrying={profile.isFetching}
          onRetry={() => void profile.refetch()}
        />
      )}
      {user && (
        <>
          {profile.isSuccess && (
            <p role="status">Статус: {userStatus(user.status)}</p>
          )}
          <UserAccess
            api={api}
            user={user}
            statusCurrent={profile.isSuccess && !profile.isFetching}
          />
        </>
      )}
      {(roles.isPending ||
        (roles.isFetching && !roles.isFetchedAfterMount)) && <LoadingState />}
      {roles.isError && (
        <ErrorState
          reason="Не удалось загрузить роли"
          retrying={roles.isFetching}
          onRetry={() => void roles.refetch()}
        />
      )}
      {roles.data && roles.isFetchedAfterMount && user && (
        <RoleEditor
          api={api}
          user={user}
          initial={roles.data}
          onSaved={() => {
            if (ownProfile)
              void client.invalidateQueries({
                queryKey: session.currentUserOptions().queryKey,
              })
          }}
        />
      )}
    </section>
  )
}
