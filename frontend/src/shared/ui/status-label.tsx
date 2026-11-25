import { Icon, Label, Text } from '@gravity-ui/uikit'
import type { ComponentProps } from 'react'

export function StatusLabel({
  text,
  icon,
  theme,
}: {
  text: string
  icon: NonNullable<ComponentProps<typeof Icon>['data']>
  theme: NonNullable<ComponentProps<typeof Label>['theme']>
}) {
  return (
    <Label
      size="m"
      theme={theme}
      icon={<Icon aria-hidden="true" data={icon} size={16} />}
    >
      <Text variant="caption-1" style={{ fontWeight: 600 }}>
        {text}
      </Text>
    </Label>
  )
}
