import { Text } from '@gravity-ui/uikit'
import { Link, useSearchParams } from 'react-router'
import { safeReturnTo } from '@/shared/auth'
import { routePaths } from '@/shared/config'

export function SessionExpiredPage() {
  const [params] = useSearchParams()
  const search = new URLSearchParams({
    return_to: safeReturnTo(params.get('return_to')),
  }).toString()
  return (
    <main>
      <Text as="h1" variant="header-1">
        Вход недоступен или сессия истекла
      </Text>
      <p>
        <Link to={`${routePaths.login}?${search}`}>Перейти ко входу</Link>
      </p>
    </main>
  )
}
