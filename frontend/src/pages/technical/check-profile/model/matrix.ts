import type {
  CheckProfileEntry,
  CheckProfileOptions,
  CheckProfileVersion,
  components,
} from '@/shared/api'
import type { ChangePreview, ChangeValidation } from '@/shared/ui'

export type ProfileDraft = components['schemas']['CheckProfileDraft']
export type ProfileRow = ProfileDraft['entries'][number]
export type ProfileMode = ProfileRow['mode']

export const analysisKinds: readonly ProfileRow['analysis_kind'][] = [
  'code',
  'sast',
  'sca',
  'secrets',
  'iac',
  'image',
  'junit',
  'coverage',
  'docs',
  'openapi',
  'sonar',
]

export const modeLabels: Record<ProfileMode, string> = {
  builtin: 'Штатный',
  import: 'Импорт',
}

// M0 profiles shipped with the product or documented by the check-profile API.
// An installation can override its catalog. The server validates the selection.
const m0Profiles = [
  ['builtin', 'code', 'go', 'go-default'],
  ['builtin', 'code', 'javascript', 'js-default'],
  ['builtin', 'code', 'python', 'python-default'],
  ['builtin', 'sast', 'go', 'opengrep-default'],
  ['builtin', 'sast', 'javascript', 'opengrep-default'],
  ['builtin', 'sast', 'python', 'opengrep-default'],
  ['builtin', 'secrets', '', 'gitleaks-default'],
  ['builtin', 'sca', '', 'trivy-default'],
  ['builtin', 'iac', '', 'trivy-default'],
  ['builtin', 'docs', '', 'git-markdown-openapi'],
  ['builtin', 'openapi', '', 'git-markdown-openapi'],
  ['import', 'code', '*', 'sonar-import'],
  ['import', 'code', '*', 'sarif-import'],
  ['import', 'sast', '*', 'sarif-import'],
  ['import', 'sca', '', 'sca-import'],
] as const

interface ProfileScope {
  language: string
  analysis_kind: string
}

function supportsLanguage(
  profile: CheckProfileOptions['profiles'][number],
  language: string,
) {
  if (profile.any_language) return language !== ''
  return profile.languages.length
    ? profile.languages.includes(language)
    : language === ''
}

export function catalogProfiles(options: CheckProfileOptions) {
  return ({ language, analysis_kind }: ProfileScope, mode: ProfileMode) =>
    options.profiles
      .filter(
        (profile) =>
          profile.mode === mode &&
          profile.analysis_kinds.includes(analysis_kind) &&
          supportsLanguage(profile, language),
      )
      .map(({ id }) => id)
}

export function addInitialRow(
  draft: ProfileDraft,
  row: ProfileRow,
  options: CheckProfileOptions,
): ProfileDraft {
  const entries = [...draft.entries, row]
  if (
    row.language &&
    options.profiles.some(
      (profile) =>
        profile.mode === 'builtin' && profile.languages.includes(row.language),
    )
  ) {
    const profilesFor = catalogProfiles(options)
    for (const analysis_kind of ['secrets', 'sca'] as const) {
      if (
        entries.some(
          (entry) =>
            entry.component_id === row.component_id &&
            entry.analysis_kind === analysis_kind,
        )
      )
        continue
      const scope = { analysis_kind, language: '' }
      const profile_id = profilesFor(scope, 'builtin')[0]
      if (profile_id)
        entries.push({
          ...scope,
          component_id: row.component_id,
          mode: 'builtin',
          profile_id,
        })
    }
  }
  return { entries }
}

export function profileOptions(entries: readonly CheckProfileEntry[]) {
  const key = (mode: string, kind: string, language: string) =>
    JSON.stringify([mode, kind, language])
  const installed = new Map<string, Set<string>>()
  for (const { mode, analysis_kind, language, profile_id } of entries) {
    const scope = key(mode, analysis_kind, language)
    const profiles = installed.get(scope) ?? new Set<string>()
    profiles.add(profile_id)
    installed.set(scope, profiles)
  }

  return (row: ProfileScope, mode: ProfileMode) => {
    const ids = new Set(
      installed.get(key(mode, row.analysis_kind, row.language)),
    )
    for (const [candidateMode, kind, language, id] of m0Profiles) {
      if (
        candidateMode === mode &&
        kind === row.analysis_kind &&
        (language === row.language || (language === '*' && row.language !== ''))
      )
        ids.add(id)
    }
    return [...ids]
  }
}

export function draftFrom(version: CheckProfileVersion): ProfileDraft | null {
  const entries: ProfileRow[] = []
  for (const {
    component_id,
    language,
    analysis_kind,
    mode,
    profile_id,
  } of version.entries) {
    const kind = analysisKinds.find((known) => known === analysis_kind)
    if (!kind || (mode !== 'builtin' && mode !== 'import')) return null
    entries.push({
      component_id,
      language,
      analysis_kind: kind,
      mode,
      profile_id,
    })
  }
  return { entries }
}

export function rowKey(
  row: Pick<ProfileRow, 'component_id' | 'language'> & {
    analysis_kind: string
  },
) {
  return JSON.stringify([row.component_id, row.language, row.analysis_kind])
}

export function rowLabel(row: Parameters<typeof rowKey>[0]) {
  return [
    row.component_id,
    row.language || 'Без языка',
    row.analysis_kind,
  ].join(' · ')
}

function selection(row: { mode: string; profile_id: string }) {
  const { mode } = row
  return `${mode === 'builtin' || mode === 'import' ? modeLabels[mode] : mode} · ${row.profile_id}`
}

function changedRows(base: CheckProfileVersion | null, draft: ProfileDraft) {
  const previous = new Map(base?.entries.map((row) => [rowKey(row), row]))
  return draft.entries.filter((row) => {
    const old = previous.get(rowKey(row))
    return old?.mode !== row.mode || old.profile_id !== row.profile_id
  })
}

export function compareDraft(
  base: CheckProfileVersion | null,
  draft: ProfileDraft,
): ChangePreview[] {
  const previous = new Map(base?.entries.map((row) => [rowKey(row), row]))
  return changedRows(base, draft).map((row) => {
    const old = previous.get(rowKey(row))
    return {
      label: rowLabel(row),
      before: old ? selection(old) : 'Нет строки',
      after: selection(row),
    }
  })
}

export function rebaseDraft(
  base: CheckProfileVersion | null,
  draft: ProfileDraft,
  current: ProfileDraft,
) {
  const changes = new Map(
    changedRows(base, draft).map((row) => [rowKey(row), row]),
  )
  const entries = current.entries.map((row) => {
    const key = rowKey(row)
    const change = changes.get(key)
    changes.delete(key)
    return change ?? row
  })
  const previous = new Set(base?.entries.map(rowKey))
  const removed: ProfileRow[] = []
  for (const [key, row] of changes) {
    if (previous.has(key)) removed.push(row)
    else entries.push(row)
  }
  return { draft: { entries }, removed }
}

const fieldReasons = {
  required: 'Обязательная строка или поле отсутствует',
  not_allowed_value: 'Профиль, режим, язык или вид анализа не поддерживается',
  unknown_reference: 'Компонент или профиль недоступен',
  duplicate: 'Строка уже есть в матрице',
  out_of_range: 'Превышено допустимое число строк',
  invalid_format: 'Недопустимый формат',
  too_long: 'Превышена допустимая длина',
  outside_scope: 'Строка вне области проекта',
  unknown_field: 'Неизвестное поле',
  unknown: 'Значение отклонено сервером',
}

export function validationMessages(
  validation: ChangeValidation,
  draft: ProfileDraft,
) {
  const entries = draft.entries
  const messages = [...validation.fields].flatMap(([pointer, errors]) => {
    const match = /^\/entries\/(\d+)(?:\/(\w+))?$/u.exec(pointer)
    const row = match ? entries[Number(match[1])] : undefined
    if (
      pointer === '/entries' &&
      errors.some(({ code }) => code === 'required')
    ) {
      const components = new Map<string, Set<string>>()
      for (const { component_id, analysis_kind } of entries) {
        const kinds = components.get(component_id) ?? new Set<string>()
        kinds.add(analysis_kind)
        components.set(component_id, kinds)
      }
      const missing = [...components].flatMap(([id, kinds]) =>
        ['secrets', 'sca']
          .filter((kind) => !kinds.has(kind))
          .map(
            (kind) =>
              `${id} · Без языка · ${kind} — обязательная проверка отсутствует`,
          ),
      )
      if (missing.length) return missing
    }
    const field = match?.[2]
    const label = row ? rowLabel(row) : 'Матрица'
    return errors.map(
      ({ code }) =>
        `${label}${field ? ` (${field})` : ''} — ${fieldReasons[code]}`,
    )
  })
  return [...new Set(messages)]
}
