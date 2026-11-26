import { Text } from '@gravity-ui/uikit'
import { useParams } from 'react-router'

export function ConnectionFormPage() {
  const { connectionId } = useParams()
  return (
    <section>
      <Text as="h1" variant="header-1">
        {connectionId ? 'Настройки подключения' : 'Новое подключение'}
      </Text>
    </section>
  )
}
