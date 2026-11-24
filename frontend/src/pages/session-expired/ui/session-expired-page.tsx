import { Container, Link, Text } from '@gravity-ui/uikit'
import { useLinkClickHandler, useSearchParams } from 'react-router'
import { safeReturnTo } from '@/shared/auth'
import { layoutSizes, routePaths } from '@/shared/config'

export function SessionExpiredPage() {
  const [params] = useSearchParams()
  const search = new URLSearchParams({
    return_to: safeReturnTo(params.get('return_to')),
  }).toString()
  const loginUrl = `${routePaths.login}?${search}`
  const onLoginClick = useLinkClickHandler(loginUrl)
  return (
    <Container
      as="main"
      maxWidth={layoutSizes.content}
      paddingBlock={{ xs: 'spacing-4', m: 'spacing-6', xl: 'spacing-8' }}
      style={{ boxSizing: 'border-box' }}
    >
      <Text as="h1" variant="header-1">
        Вход недоступен или сессия истекла
      </Text>
      <p>
        <Link href={loginUrl} onClick={onLoginClick}>
          Перейти ко входу
        </Link>
      </p>
    </Container>
  )
}
