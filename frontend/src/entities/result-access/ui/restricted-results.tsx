import type { components, OpenCodes } from '@/shared/api'

type Schema = components['schemas']

export function RestrictedResults({
  access_limited = false,
  redactions,
  access_notes,
}: {
  access_limited?: boolean
  redactions?: Readonly<Schema['Redactions']>
  access_notes?: readonly OpenCodes<Schema['AccessNote']>[]
}) {
  if (!access_limited && !redactions?.length && !access_notes?.length)
    return null
  return <p>Есть закрытые результаты.</p>
}
