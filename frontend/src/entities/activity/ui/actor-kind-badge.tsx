import { Person, Cpu, Gear, CircleQuestion } from '@gravity-ui/icons'
import type { components } from '@/shared/api'
import { contractLabel } from '@/shared/config'
import { ActivityBadge } from './activity-badge'

type ActorKind = components['schemas']['ActorRef']['kind']

const labels = {
  human: 'Человек',
  bot: 'Бот',
  system: 'Система',
  unknown: 'Вид участника неизвестен',
} satisfies Record<ActorKind, string>

const icons: Readonly<Record<string, typeof Person>> = {
  human: Person,
  bot: Cpu,
  system: Gear,
  unknown: CircleQuestion,
} satisfies Record<ActorKind, typeof Person>

export function ActorKindBadge({ kind }: { kind: string }) {
  const icon = Object.hasOwn(icons, kind) ? icons[kind] : undefined
  return (
    <ActivityBadge icon={icon ?? CircleQuestion}>
      {contractLabel(labels, kind)}
    </ActivityBadge>
  )
}
