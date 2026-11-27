import {
  CircleCheck,
  CircleQuestion,
  CircleXmark,
  TriangleExclamation,
} from '@gravity-ui/icons'
import { Flex } from '@gravity-ui/uikit'
import type { components, OpenCodes } from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'
import { ContractReason, StatusLabel, StatusTime } from '@/shared/ui'

type Capability = components['schemas']['Capability']

const badges = {
  verified: { icon: CircleCheck, theme: 'success' },
  partial: { icon: TriangleExclamation, theme: 'warning' },
  missing: { icon: CircleXmark, theme: 'danger' },
} as const satisfies Record<
  Capability['state'],
  { icon: typeof CircleCheck; theme: string }
>
const unknown = { icon: CircleQuestion, theme: 'unknown' } as const

function capabilityBadge(state: string) {
  switch (state) {
    case 'verified':
    case 'partial':
    case 'missing':
      return badges[state]
    default:
      return unknown
  }
}

export function CapabilityList({
  capabilities,
  unchecked = [],
}: {
  capabilities: readonly OpenCodes<Capability>[]
  unchecked?: readonly string[]
}) {
  return (
    <ul>
      {capabilities.map(({ capability, state, reason_code, checked_at }) => (
        <li key={capability}>
          <Flex direction="column" alignItems="flex-start" gap="spacing-2">
            <Flex wrap alignItems="center" gap="spacing-2">
              <strong>
                {contractLabel(adminLabels.capability, capability)}
              </strong>
              <StatusLabel
                {...capabilityBadge(state)}
                text={contractLabel(adminLabels.capabilityState, state)}
              />
            </Flex>
            <span>
              Время проверки: <StatusTime value={checked_at} />
            </span>
            {reason_code !== null ? (
              <ContractReason
                code={reason_code}
                labels={adminLabels.capabilityReason}
              />
            ) : (
              state !== 'verified' && <span>Причина не указана</span>
            )}
          </Flex>
        </li>
      ))}
      {unchecked.map((capability) => (
        <li key={capability}>
          <Flex wrap alignItems="center" gap="spacing-2">
            <strong>{contractLabel(adminLabels.capability, capability)}</strong>
            <StatusLabel {...unknown} text="не проверено" />
          </Flex>
        </li>
      ))}
    </ul>
  )
}
