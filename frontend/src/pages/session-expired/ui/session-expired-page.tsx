import { Clock } from '@gravity-ui/icons'
import {
  Alert,
  Button,
  Card,
  Container,
  Flex,
  Icon,
  Text,
} from '@gravity-ui/uikit'
import { useLayoutEffect, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { createLoginPageUrl, safeReturnTo } from '@/shared/auth'
import type { SessionController } from '@/shared/auth'
import { layoutSizes } from '@/shared/config'

export function SessionExpiredPage({
  session,
}: {
  session: SessionController
}) {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const headingRef = useRef<HTMLHeadingElement>(null)
  const returnTo = safeReturnTo(params.get('return_to'))
  const organization = session.getLoginOrganization()
  useLayoutEffect(() => {
    headingRef.current?.focus()
  }, [])
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
              Сессия завершилась
            </Text>
            <div role="alert">
              <Alert
                theme="warning"
                view="filled"
                message="После входа вы вернётесь на прежнюю страницу."
                icon={
                  <Icon
                    data={Clock}
                    size={20}
                    style={{ verticalAlign: 'middle' }}
                  />
                }
              />
            </div>
            <Text variant="body-short">Форма не отправится автоматически.</Text>
            <Text variant="body-short" color="secondary">
              Секретные поля заполните заново, файл выберите повторно.
            </Text>
            <Button
              view="action"
              width="max"
              onClick={() => {
                void navigate(createLoginPageUrl(organization, returnTo), {
                  replace: true,
                })
              }}
            >
              Войти снова
            </Button>
            <Button
              view="outlined"
              width="max"
              onClick={() => {
                void navigate(createLoginPageUrl(organization), {
                  replace: true,
                })
              }}
            >
              Вернуться к началу
            </Button>
          </Flex>
        </Card>
      </Flex>
    </Container>
  )
}
