import { Icon, Label, Text } from '@gravity-ui/uikit'
import type { ComponentProps } from 'react'

export function ActivityBadge({
  icon,
  children,
  theme = 'normal',
}: {
  icon: ComponentProps<typeof Icon>['data']
  children: string
  theme?: ComponentProps<typeof Label>['theme']
}) {
  return (
    <Label
      size="m"
      theme={theme}
      icon={<Icon aria-hidden="true" data={icon} size={16} />}
    >
      <Text variant="body-1" style={{ fontWeight: 600 }}>
        {children}
      </Text>
    </Label>
  )
}
