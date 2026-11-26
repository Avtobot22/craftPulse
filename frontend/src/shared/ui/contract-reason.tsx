import { TechnicalValue } from './technical-value'

export function ContractReason({
  code,
  labels,
}: {
  code: string
  labels: Readonly<Record<string, string>>
}) {
  const label = Object.hasOwn(labels, code) ? labels[code] : undefined
  return <TechnicalValue label={label ?? 'Неизвестная причина'} value={code} />
}
