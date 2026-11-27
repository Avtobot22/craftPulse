import { Text } from '@gravity-ui/uikit'
import { useLocation } from 'react-router'
import { readTaskOrigin } from '@/shared/config'
import { InspectionBack, TaskReturnLink } from '@/shared/ui'

export function UnavailablePage({
  correlationId,
}: {
  correlationId?: string | undefined
}) {
  const state: unknown = useLocation().state
  const origin = readTaskOrigin(state)
  return (
    <section>
      <Text as="h1" variant="header-1">
        Объект или область недоступны
      </Text>
      {correlationId && <p>Код обращения: {correlationId}</p>}
      {origin ? <TaskReturnLink origin={origin} /> : <InspectionBack />}
    </section>
  )
}
