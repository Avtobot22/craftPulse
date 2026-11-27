import type { components, OpenCodes } from '@/shared/api'

export type IngestConflict = OpenCodes<components['schemas']['IngestConflict']>
type Resolution =
  components['schemas']['IngestConflictResolutionInput']['resolution']

export const resolutions = {
  kept_existing: {
    label: 'Оставить принятую запись',
    consequence: 'Принятая запись останется без изменений.',
  },
  accepted_new: {
    label: 'Принять новую запись',
    consequence:
      'Будет создана новая версия знания и пересчитаны затронутые показатели.',
  },
  source_corrected: {
    label: 'Источник исправлен',
    consequence: 'Источник будет перечитан.',
  },
} satisfies Record<Resolution, { label: string; consequence: string }>

export const resolutionOptions = Object.keys(resolutions).filter(isResolution)

export function isResolution(value: string): value is Resolution {
  return Object.hasOwn(resolutions, value)
}
