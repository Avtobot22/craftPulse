import { Button } from '@gravity-ui/uikit'
import type { CheckProfileVersion } from '@/shared/api'
import { ContractReason } from '@/shared/ui'
import { modeLabels, profileOptions, rowKey, rowLabel } from '../model/matrix'
import type { ProfileDraft, ProfileMode, ProfileRow } from '../model/matrix'

const uncoveredReasons = {
  language_unsupported: 'Язык не поддерживается',
  profile_missing: 'Профиль для компонента не настроен',
  profile_unavailable: 'Профиль недоступен в каталоге',
}

export function MatrixTable({
  version,
  draft,
  initialProfiles,
  onRemove,
  onChange,
}: {
  version: CheckProfileVersion | null
  draft: ProfileDraft | null
  initialProfiles: ReturnType<typeof profileOptions> | undefined
  onRemove: ((index: number) => void) | undefined
  onChange: (
    index: number,
    selection: Pick<ProfileRow, 'mode' | 'profile_id'>,
  ) => void
}) {
  const confirmedProfiles = profileOptions(version?.entries ?? [])
  const rows = draft?.entries ?? version?.entries ?? []
  const confirmed = new Map(version?.entries.map((row) => [rowKey(row), row]))
  const hasNewRows = rows.some((row) => !confirmed.has(rowKey(row)))
  return (
    <>
      <p>
        Импорт одного вида анализа не отключает проверки секретов (secrets) и
        зависимостей (sca).
      </p>
      {rows.length ? (
        <div
          className="cp-check-matrix__scroll"
          role="region"
          aria-label="Строки матрицы"
          tabIndex={0}
        >
          <table
            className="cp-check-matrix__table"
            aria-label="Матрица проверок"
          >
            <thead>
              <tr>
                {[
                  'Компонент',
                  'Язык',
                  'Вид анализа',
                  'Режим',
                  'Профиль',
                  'Версии инструментов',
                  'Хэш правил',
                  ...(hasNewRows ? ['Действия'] : []),
                ].map((label) => (
                  <th key={label} scope="col">
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, index) => {
                const original = confirmed.get(rowKey(row))
                const profilesFor =
                  !original && initialProfiles
                    ? initialProfiles
                    : confirmedProfiles
                const choice = draft?.entries[index]
                const label = rowLabel(row)
                const modes: readonly ProfileMode[] = ['builtin', 'import']
                const profiles = {
                  builtin: profilesFor(row, 'builtin'),
                  import: profilesFor(row, 'import'),
                }
                if (
                  choice &&
                  !profiles[choice.mode].includes(choice.profile_id)
                ) {
                  profiles[choice.mode].push(choice.profile_id)
                }
                const changed =
                  choice &&
                  (choice.mode !== original?.mode ||
                    choice.profile_id !== original.profile_id)
                const tools = Object.entries(original?.tool_versions ?? {})
                  .map(([name, version]) => `${name} ${version}`)
                  .join(', ')
                return (
                  <tr key={rowKey(row)}>
                    <th scope="row" data-label="Компонент">
                      {row.component_id}
                    </th>
                    <td data-label="Язык">{row.language || 'Без языка'}</td>
                    <td data-label="Вид анализа">{row.analysis_kind}</td>
                    <td data-label="Режим">
                      {choice ? (
                        <select
                          aria-label={`Режим · ${label}`}
                          value={choice.mode}
                          onChange={(event) => {
                            const mode = event.currentTarget.value
                            if (mode !== 'builtin' && mode !== 'import') return
                            const profile_id =
                              mode === original?.mode
                                ? original.profile_id
                                : profiles[mode][0]
                            if (profile_id)
                              onChange(index, { mode, profile_id })
                          }}
                        >
                          {modes
                            .filter((mode) => profiles[mode].length > 0)
                            .map((mode) => (
                              <option value={mode} key={mode}>
                                {modeLabels[mode]}
                              </option>
                            ))}
                        </select>
                      ) : row.mode === 'builtin' || row.mode === 'import' ? (
                        modeLabels[row.mode]
                      ) : (
                        row.mode
                      )}
                    </td>
                    <td data-label="Профиль">
                      {choice ? (
                        <select
                          aria-label={`Профиль · ${label}`}
                          value={choice.profile_id}
                          onChange={(event) => {
                            onChange(index, {
                              mode: choice.mode,
                              profile_id: event.currentTarget.value,
                            })
                          }}
                        >
                          {profiles[choice.mode].map((id) => (
                            <option value={id} key={id}>
                              {id}
                            </option>
                          ))}
                        </select>
                      ) : (
                        row.profile_id
                      )}
                    </td>
                    <td data-label="Версии инструментов">
                      {changed ? 'После подтверждения' : tools || 'Нет данных'}
                    </td>
                    <td data-label="Хэш правил">
                      {changed
                        ? 'После подтверждения'
                        : (original?.rules_hash ?? 'Нет данных')}
                    </td>
                    {hasNewRows && (
                      <td data-label="Действия">
                        {!original && onRemove && (
                          <Button
                            type="button"
                            aria-label={`Удалить строку · ${label}`}
                            onClick={() => {
                              onRemove(index)
                            }}
                          >
                            Удалить
                          </Button>
                        )}
                      </td>
                    )}
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <p>В матрице пока нет строк.</p>
      )}
      {version && (
        <section aria-label="Непокрытые области">
          <h2>Непокрытые области</h2>
          {version.uncovered.length ? (
            <ul>
              {version.uncovered.map(
                ({ component_id, language, reason_code }, index) => (
                  <li key={index}>
                    {component_id} · {language || 'Без языка'} ·{' '}
                    <ContractReason
                      code={reason_code}
                      labels={uncoveredReasons}
                    />
                  </li>
                ),
              )}
            </ul>
          ) : (
            <p>Непокрытых областей нет.</p>
          )}
        </section>
      )}
    </>
  )
}
