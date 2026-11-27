import type { CurrentUser } from './current-user'

export function canManageDocumentation(
  current: CurrentUser,
  serviceId: string,
) {
  return (
    current.actions.includes('docs.manage') &&
    current.roles.some(
      (role) => role.role === 'service_owner' && role.service_id === serviceId,
    )
  )
}
