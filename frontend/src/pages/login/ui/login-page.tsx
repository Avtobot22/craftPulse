import { CircleXmark, LogoGitlab } from '@gravity-ui/icons'
import {
  Alert,
  Button,
  Card,
  Container,
  Flex,
  Icon,
  Link,
  Text,
} from '@gravity-ui/uikit'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import {
  createLoginPageUrl,
  isOrganizationSlug,
  navigateToLogin,
  safeReturnTo,
} from '@/shared/auth'
import type { SessionController } from '@/shared/auth'
import { layoutSizes } from '@/shared/config'
import { loginErrorMessage } from '../model/login-errors'

export function LoginPage({ session }: { session: SessionController }) {
  const { orgSlug: routeSlug } = useParams<'orgSlug'>()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const headingRef = useRef<HTMLHeadingElement>(null)
  const started = useRef(false)
  const [redirecting, setRedirecting] = useState(false)
  const organization =
    routeSlug === undefined
      ? session.getLoginOrganization()
      : isOrganizationSlug(routeSlug)
        ? routeSlug
        : null
  const returnTo = safeReturnTo(params.get('return_to'))
  const errorCode = params.get('login_error')
  const error = errorCode === null ? null : loginErrorMessage(errorCode)
  useLayoutEffect(() => {
    headingRef.current?.focus()
  }, [errorCode, routeSlug])
  useEffect(() => {
    started.current = false
  }, [errorCode, routeSlug])
  useEffect(() => {
    function onPageShow() {
      started.current = false
      setRedirecting(false)
    }
    window.addEventListener('pageshow', onPageShow)
    return () => {
      window.removeEventListener('pageshow', onPageShow)
    }
  }, [])

  function beginLogin() {
    if (!organization || started.current) return
    started.current = true
    setRedirecting(true)
    navigateToLogin(organization, returnTo)
  }
  function toStart() {
    started.current = false
    setRedirecting(false)
    void navigate(createLoginPageUrl(organization), { replace: true })
  }
  return (
    <Container
      as="main"
      maxWidth={layoutSizes.content}
      paddingBlock={{ xs: 'spacing-4', m: 'spacing-6', xl: 'spacing-8' }}
      style={{ boxSizing: 'border-box' }}
    >
      <Flex
        minHeight="calc(100dvh - 64px)"
        alignItems="center"
        justifyContent="center"
      >
        <Card view="outlined" style={{ width: '100%', maxWidth: 560 }}>
          <Flex direction="column" padding="spacing-6" gap="spacing-4">
            <Text
              ref={headingRef}
              as="h1"
              tabIndex={-1}
              variant="header-1"
              style={{ margin: 0 }}
            >
              {error ? 'Не удалось войти' : 'Вход'}
            </Text>
            {error ? (
              <div role="alert">
                <Alert
                  theme="danger"
                  view="filled"
                  title={error.title}
                  message={error.message}
                  icon={
                    <Icon
                      data={CircleXmark}
                      size={20}
                      color="danger-heavy"
                      style={{ verticalAlign: 'middle' }}
                    />
                  }
                />
              </div>
            ) : (
              <Text variant="body-short">
                Вы перейдёте на страницу входа GitLab вашей организации.
              </Text>
            )}
            {!organization && (
              <Text variant="body-short">
                Откройте ссылку входа, полученную у администратора организации.
              </Text>
            )}
            {error?.retry === false ? (
              <Button view="action" width="max" onClick={toStart}>
                Вернуться к началу
              </Button>
            ) : (
              <Button
                view="action"
                width="max"
                disabled={!organization || redirecting}
                loading={redirecting}
                onClick={beginLogin}
              >
                <Icon data={LogoGitlab} size={20} />
                {error?.action ?? 'Войти через GitLab'}
              </Button>
            )}
            {error?.retry && (
              <Button view="outlined" width="max" onClick={toStart}>
                Вернуться к началу
              </Button>
            )}
            <Text as="span" role="status" variant="body-short">
              {redirecting ? 'Переходим к провайдеру входа' : ''}
            </Text>
            <Text variant="body-short">
              <Link href="#login-help">Помощь со входом</Link>
            </Text>
            <Text
              as="p"
              id="login-help"
              variant="body-short"
              color="secondary"
              style={{ margin: 0 }}
            >
              За доступом к GitLab обратитесь к администратору вашей
              организации.
            </Text>
          </Flex>
        </Card>
      </Flex>
    </Container>
  )
}
