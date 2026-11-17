import { Text } from '@gravity-ui/uikit'
export function UnavailablePage({
  correlationId,
}: {
  correlationId?: string | undefined
}) {
  return (
    <section>
      <Text as="h1" variant="header-1">
        Объект или область недоступны
      </Text>
      {correlationId && <p>Код обращения: {correlationId}</p>}
    </section>
  )
}
