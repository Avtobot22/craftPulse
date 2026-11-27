import { CircleCheck } from '@gravity-ui/icons'
import { Alert, Flex, Icon, Text } from '@gravity-ui/uikit'
import { Link, useOutletContext } from 'react-router'
import type { CurrentUser } from '@/shared/auth'
import { routePaths } from '@/shared/config'

export function WelcomePage() {
  const { organization } = useOutletContext<CurrentUser>()
  return (
    <Flex
      as="section"
      direction="column"
      alignItems="flex-start"
      gap="spacing-4"
    >
      <Text as="h1" variant="header-1">
        Результат первого входа
      </Text>
      <Alert
        theme="success"
        icon={<Icon data={CircleCheck} size={20} />}
        title="Вам назначена роль администратора"
      />
      <div style={{ overflowWrap: 'anywhere', minWidth: 0, maxWidth: '100%' }}>
        <div>Организация: {organization.name}</div>
        <div>Ваша роль: Администратор</div>
      </div>
      <div>
        <Text as="h2" variant="subheader-2">
          Настройка организации
        </Text>
        <ol>
          <li aria-current="step">
            <Link to={routePaths.organization}>
              Описать структуру организации
            </Link>
          </li>
          <li>Подключить источник</li>
          <li>Загрузить историю</li>
          <li>Настроить рабочий процесс</li>
          <li>Открыть отчёт</li>
        </ol>
      </div>
    </Flex>
  )
}
