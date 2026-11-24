import {
  CircleCheck,
  CircleQuestion,
  CircleMinus,
  TriangleExclamation,
  CircleXmark,
} from '@gravity-ui/icons'
import { Flex, Icon, Label, Text } from '@gravity-ui/uikit'

const states = {
  PROVISIONAL: {
    text: 'частично',
    icon: TriangleExclamation,
    theme: 'warning',
  },
  BOUNDED: { text: 'известна граница', icon: CircleMinus, theme: 'info' },
  NOT_EVALUATED: { text: 'не проверено', icon: CircleXmark, theme: 'normal' },
  UNKNOWN: { text: 'неизвестно', icon: CircleQuestion, theme: 'unknown' },
  NOT_APPLICABLE: { text: 'неприменимо', icon: CircleMinus, theme: 'normal' },
  ASSESSED: { text: 'оценено', icon: CircleCheck, theme: 'success' },
} as const
export type DataState = keyof typeof states
export interface DataStateBadgeProps {
  state: DataState
  reason?: string
}
export function DataStateBadge({ state, reason }: DataStateBadgeProps) {
  const { text, icon, theme } = states[state]
  const badge = (
    <Label size="m" theme={theme} icon={<Icon data={icon} size={16} />}>
      <Text variant="caption-1" style={{ fontWeight: 600 }}>
        {text}
      </Text>
    </Label>
  )
  if (!reason) return badge
  return (
    <Flex inline alignItems="center" wrap gap="spacing-2">
      {badge}
      <Text variant="caption-1">{reason}</Text>
    </Flex>
  )
}
