import {
  CircleCheck,
  CircleMinus,
  CircleQuestion,
  CircleXmark,
  Clock,
  TriangleExclamation,
} from '@gravity-ui/icons'
import type { components } from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'
import { StatusLabel } from '@/shared/ui'

const badges = {
  quarantined: { icon: Clock, theme: 'info' },
  accepted: { icon: CircleCheck, theme: 'success' },
  partial: { icon: TriangleExclamation, theme: 'warning' },
  rejected: { icon: CircleXmark, theme: 'danger' },
  duplicate: { icon: CircleMinus, theme: 'normal' },
} as const satisfies Record<
  components['schemas']['Receipt']['status'],
  { icon: typeof CircleCheck; theme: string }
>
const unknown = { icon: CircleQuestion, theme: 'unknown' } as const

function receiptBadge(status: string) {
  switch (status) {
    case 'quarantined':
    case 'accepted':
    case 'partial':
    case 'rejected':
    case 'duplicate':
      return badges[status]
    default:
      return unknown
  }
}

export function ReceiptStatusBadge({ status }: { status: string }) {
  return (
    <StatusLabel
      {...receiptBadge(status)}
      text={contractLabel(adminLabels.receiptStatus, status)}
    />
  )
}
