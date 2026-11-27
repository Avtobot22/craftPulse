import type { components, OpenCodes } from '@/shared/api'
import { adminLabels, contractLabel } from '@/shared/config'

export type ConnectionProbe = OpenCodes<
  components['schemas']['ConnectionProbe']
>

export const capabilityGroups = [
  { title: 'Текущее состояние', names: ['read_current'] },
  {
    title: 'История',
    names: [
      'read_status_history',
      'read_field_history',
      'read_resolution_history',
      'read_sprint_history',
      'read_deletions',
    ],
  },
  { title: 'Связи', names: ['read_links'] },
  { title: 'Права', names: ['read_permissions'] },
  {
    title: 'Публикация и запись',
    names: ['write_mr_comment', 'write_tracker_issue'],
  },
  { title: 'Вебхуки', names: ['webhook'] },
] satisfies {
  title: string
  names: components['schemas']['Capability']['capability'][]
}[]

export const groupedCapabilities = new Set<string>(
  capabilityGroups.flatMap(({ names }) => names),
)

export function capabilityNames(names: string[]) {
  return (
    names
      .map((name) => contractLabel(adminLabels.capability, name))
      .join(', ') || 'Нет'
  )
}

export const probeRemedies = {
  edition_mismatch: 'Воспользуйтесь универсальным импортом.',
  auth_failed:
    'Замените учётные данные в форме, сохраните их и повторите проверку.',
  outside_network_scope:
    'Проверьте базовый адрес и разрешённую сетевую область.',
  certificate_untrusted:
    'Проверьте адрес и настройте доверие к сертификату источника.',
  timeout: 'Проверьте адрес источника или повторите проверку позже.',
  adapter_unavailable: 'Повторите проверку позже.',
  contract_unsupported:
    'Уточните поддерживаемую версию у администратора установки и повторите проверку.',
  configuration_changed:
    'Сохраните актуальные настройки и запустите новую проверку.',
} satisfies Record<keyof typeof adminLabels.probeReason, string>
