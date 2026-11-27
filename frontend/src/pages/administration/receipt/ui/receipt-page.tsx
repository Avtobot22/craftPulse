import { Flex, Text } from '@gravity-ui/uikit'
import { Link, useOutletContext } from 'react-router'
import type { CurrentUser } from '@/shared/auth'
import { adminLinks } from '@/shared/config'

export function ReceiptPage() {
  const { actions } = useOutletContext<CurrentUser>()
  return (
    <Flex as="section" direction="column" gap="spacing-4">
      <Text as="h1" variant="header-1">
        Квитанция приёма
      </Text>
      {actions.includes('sync.read') && (
        <Link to={adminLinks.ingestConflicts()}>
          Конфликты приёма организации
        </Link>
      )}
    </Flex>
  )
}
