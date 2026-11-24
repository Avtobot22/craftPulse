import { CircleCheck, CircleXmark } from '@gravity-ui/icons'
import { Icon, Text, useToaster } from '@gravity-ui/uikit'
import type { ToastProps } from '@gravity-ui/uikit'

export interface NotificationOptions {
  name: string
  message: string
  actions?: ToastProps['actions']
}
/** Application defaults for the native UIKit toaster. */
export function useNotifications() {
  const toaster = useToaster()
  return {
    success({ name, message, actions }: NotificationOptions) {
      const hasActions =
        typeof actions === 'function' || (actions?.length ?? 0) > 0
      toaster.add({
        name,
        content: <Text variant="caption-1">{message}</Text>,
        theme: 'success',
        renderIcon: () => <Icon data={CircleCheck} size={20} />,
        autoHiding: hasActions ? false : 8000,
        ...(actions ? { actions } : {}),
      })
    },
    error({ name, message, actions }: NotificationOptions) {
      toaster.add({
        name,
        content: <Text variant="caption-1">{message}</Text>,
        theme: 'danger',
        renderIcon: () => <Icon data={CircleXmark} size={20} />,
        autoHiding: false,
        ...(actions ? { actions } : {}),
      })
    },
  }
}
