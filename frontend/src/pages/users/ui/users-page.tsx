import { Text } from '@gravity-ui/uikit'
import { useOutletContext, useParams } from 'react-router'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser, SessionController } from '@/shared/auth'
import { UnavailableRedirect } from '@/shared/ui'
import { UserDetails } from './user-details'
import { UserList } from './user-list'
import './users.css'

export function UsersPage({
  api,
  session,
}: {
  api: ApiClient
  session: SessionController
}) {
  const { userId } = useParams()
  const current = useOutletContext<CurrentUser>()

  if (!current.actions.includes('users.manage'))
    return <UnavailableRedirect forbidden />

  return (
    <section>
      <Text as="h1" variant="header-1">
        Пользователи и роли
      </Text>
      <div className={userId ? 'cp-users cp-users_split' : 'cp-users'}>
        <UserList api={api} selected={userId} />
        {userId && (
          <UserDetails
            key={userId}
            api={api}
            session={session}
            userId={userId}
            ownProfile={userId === current.user.id}
          />
        )}
      </div>
    </section>
  )
}
