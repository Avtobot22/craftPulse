import { Flex, Text } from '@gravity-ui/uikit'
import { Link, useOutletContext } from 'react-router'
import type { CurrentUser } from '@/shared/auth'
import { adminLinks } from '@/shared/config'

export function ReceiptJournalPage() {
  const { actions } = useOutletContext<CurrentUser>()
  return (
    <Flex as="section" direction="column" gap="spacing-4">
      <Text as="h1" variant="header-1">
        Журнал приёма
      </Text>
      {actions.includes('sync.read') && (
        <Link to={adminLinks.ingestConflicts()}>
          Конфликты приёма организации
        </Link>
      )}
    </Flex>
  )
}
