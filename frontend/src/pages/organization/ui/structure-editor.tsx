import { Button, TextInput } from '@gravity-ui/uikit'
import { useId, useLayoutEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import type { SubmitEvent } from 'react'
import type { ApiClient, ApiResponse } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ChangeConfirmationDialog, UnsavedChangesDialog } from '@/shared/ui'
import type { Structure } from '../api/structure'
import { recordKey, readSelection, tabFor, tabs } from '../model/selection'
import {
  describeRow,
  editRow,
  noInterval,
  sections,
  statuses,
} from '../model/structure-draft'
import type {
  Draft,
  LinkSection,
  NamedSection,
  Section,
} from '../model/structure-draft'
import type { Issue } from '../model/structure-errors'
import { useStructureDraft } from '../model/use-structure-draft'
import { LinkTables } from './link-tables'
import type { Catalog } from './link-tables'
import { RecordEditor } from './record-editor'
import { StructureErrors } from './structure-errors'
import { fieldId } from './structure-fields'
import { StructureTable } from './structure-table'

export function StructureEditor({
  api,
  initial,
  catalog,
  onSaved,
}: {
  api: ApiClient
  initial: ApiResponse<Structure>
  catalog: Catalog
  onSaved: (result: ApiResponse<Structure>) => void
}) {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const [filter, setFilter] = useState('')
  const [discardRequested, setDiscardRequested] = useState(false)
  const [focusTarget, setFocusTarget] = useState<string>()
  const formRef = useRef<HTMLFormElement>(null)
  const opener = useRef<HTMLButtonElement>(null)
  const filterId = useId()
  const externalNames = new Map(
    [...catalog.areas, ...catalog.people].map(({ id, name }) => [id, name]),
  )
  const editor = useStructureDraft(
    api,
    initial,
    (id) => externalNames.get(id) ?? id,
    onSaved,
  )
  const { draft, snapshot } = editor
  const { tab, section, index } = readSelection(params, draft)
  const named = new Map(
    [...draft.teams, ...draft.projects, ...draft.services].map(
      ({ id, name }) => [id, name],
    ),
  )
  const nameOf = (id: string) =>
    named.get(id) ?? externalNames.get(id) ?? (id || 'Не выбрано')

  useLayoutEffect(() => {
    if (!focusTarget) return
    const field = document.getElementById(focusTarget)
    if (field && !field.matches(':disabled')) field.focus()
    else document.getElementById(fieldId(section))?.focus()
  }, [focusTarget, section, index])

  function select(nextSection: Section, record?: string, field?: string) {
    const next = new URLSearchParams({
      tab: tabFor(nextSection),
      section: nextSection,
    })
    if (record !== undefined) next.set('record', record)
    setParams(next, { preventScrollReset: true })
    const target = fieldId(nextSection, field)
    setFocusTarget(target)
    document.getElementById(target)?.focus()
  }

  function add<K extends Section>(
    kind: K,
    row: Draft[K][number],
    field: string,
  ) {
    const position = draft[kind].length
    editor.edit(editRow(draft, kind, position, row))
    select(kind, recordKey(row, position), field)
  }

  function addLink(
    kind: LinkSection,
    owner?: { kind: 'teams' | 'projects'; id: string },
  ) {
    const team_id = owner?.kind === 'teams' ? owner.id : ''
    const project_id = owner?.kind === 'projects' ? owner.id : ''
    if (kind === 'team_projects') {
      add(
        kind,
        { team_id, project_id, ...noInterval },
        team_id ? 'project_id' : 'team_id',
      )
    }
    if (kind === 'team_members')
      add(
        kind,
        { team_id, person_id: '', ...noInterval },
        team_id ? 'person_id' : 'team_id',
      )
    if (kind === 'project_areas')
      add(
        kind,
        { project_id, area_id: '', ...noInterval },
        project_id ? 'area_id' : 'project_id',
      )
  }

  function selectLink(kind: LinkSection, position: number) {
    select(
      kind,
      String(position),
      kind === 'project_areas' ? 'project_id' : 'team_id',
    )
  }

  function remove() {
    editor.edit(editRow(draft, section, index))
    const neighbor = draft[section][index + 1] ?? draft[section][index - 1]
    const position = index + 1 < draft[section].length ? index : index - 1
    select(section, neighbor ? recordKey(neighbor, position) : undefined)
  }

  function showIssue({ section, index, field }: Issue) {
    if (!section) return
    const row = index === undefined ? undefined : draft[section][index]
    select(
      section,
      row && index !== undefined ? recordKey(row, index) : undefined,
      field,
    )
  }

  const matches = (text: string) =>
    text.toLocaleLowerCase().includes(filter.trim().toLocaleLowerCase())
  const ownerId =
    section === 'teams' || section === 'projects'
      ? draft[section][index]?.id
      : null
  const owner =
    ownerId && (section === 'teams' || section === 'projects')
      ? { kind: section, id: ownerId }
      : undefined
  const canAddComponent =
    catalog.areaProblem ??
    (!catalog.areas.length
      ? 'Источник не настроен'
      : !catalog.areas.some(({ kind }) => kind === 'repository')
        ? 'Подключите источник репозиториев'
        : !draft.services.some(({ id }) => id !== null)
          ? 'Сначала сохраните запись'
          : undefined)

  function namedList(kind: NamedSection) {
    return (
      <StructureTable
        key={kind + ':' + filter}
        title={sections[kind]}
        columns={['Имя', 'Статус']}
        activeIndex={section === kind ? index : undefined}
        onAdd={() => {
          add(kind, { id: null, name: '', status: 'active' }, 'name')
        }}
        empty={filter ? 'Ничего не найдено' : 'Записей пока нет'}
        rows={draft[kind].flatMap((row, position) => {
          if (!matches(row.name)) return []
          const name = row.name || 'Без имени'
          return [
            {
              index: position,
              cells: [name, statuses[row.status]],
              actionLabel: 'Изменить ' + name,
              onEdit: () => {
                select(kind, recordKey(row, position), 'name')
              },
            },
          ]
        })}
      />
    )
  }

  function componentList(proposed: boolean) {
    return (
      <StructureTable
        key={String(proposed) + ':' + filter}
        title={proposed ? 'Предложения из манифестов' : 'Компоненты'}
        columns={['Сервис', 'Репозиторий', 'Префикс пути', 'Статус']}
        activeIndex={section === 'components' ? index : undefined}
        unavailable={proposed ? undefined : canAddComponent}
        onAdd={
          proposed
            ? undefined
            : () => {
                add(
                  'components',
                  {
                    id: null,
                    service_id: '',
                    repository_area_id: '',
                    path_prefix: '',
                    status: 'confirmed',
                    origin: 'manual',
                  },
                  'service_id',
                )
              }
        }
        rows={draft.components.flatMap((row, position) => {
          if (
            (row.status === 'proposed') !== proposed ||
            !matches(describeRow(row, nameOf))
          )
            return []
          const path = row.path_prefix || 'Весь репозиторий'
          return [
            {
              index: position,
              cells: [
                nameOf(row.service_id),
                nameOf(row.repository_area_id),
                path,
                statuses[row.status],
              ],
              actionLabel: (proposed ? 'Исправить путь ' : 'Изменить ') + path,
              editText: proposed ? 'Исправить путь' : 'Изменить',
              actions: proposed && (
                <>
                  <Button
                    type="button"
                    view="flat"
                    aria-label={'Подтвердить ' + path}
                    disabled={Boolean(canAddComponent)}
                    onClick={() => {
                      editor.edit(
                        editRow(draft, 'components', position, {
                          ...row,
                          status: 'confirmed',
                        }),
                      )
                      select('components', recordKey(row, position), 'status')
                    }}
                  >
                    Подтвердить
                  </Button>
                  <Button
                    type="button"
                    view="flat"
                    aria-label={'Отклонить ' + path}
                    onClick={() => {
                      editor.edit(
                        editRow(draft, 'components', position, {
                          ...row,
                          status: 'rejected',
                        }),
                      )
                      select('components', recordKey(row, position), 'status')
                    }}
                  >
                    Отклонить
                  </Button>
                </>
              ),
              onEdit: () => {
                select('components', recordKey(row, position), 'path_prefix')
              },
            },
          ]
        })}
      />
    )
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    editor.openConfirmation()
  }

  function discard() {
    editor.discard()
    setDiscardRequested(false)
  }

  return (
    <>
      <p role="status">Сохранённая версия структуры: {snapshot.data.version}</p>
      {editor.dirty && (
        <p>Изменения в черновике вступят в силу после подтверждения.</p>
      )}
      {!draft.teams.length && !draft.projects.length && (
        <p>Добавьте команду и проект, чтобы настроить срезы отчётов.</p>
      )}
      <form ref={formRef} className="cp-org" onSubmit={submit} noValidate>
        <nav className="cp-org__bar" aria-label="Разделы структуры">
          {tabs.map((item) => (
            <Button
              key={item.id}
              type="button"
              view={tab.id === item.id ? 'outlined-action' : 'flat'}
              aria-current={tab.id === item.id ? 'page' : undefined}
              onClick={() => {
                setFilter('')
                select(item.section)
              }}
            >
              {item.label}
            </Button>
          ))}
        </nav>
        <StructureErrors issues={editor.issues} showIssue={showIssue} />
        {tab.id !== 'links' && (
          <div className="cp-org__field">
            <label htmlFor={filterId}>Поиск в списке</label>
            <TextInput id={filterId} value={filter} onUpdate={setFilter} />
          </div>
        )}
        <div className="cp-org__workspace">
          <div className="cp-org__lists">
            {tab.id === 'links' ? (
              <LinkTables
                draft={draft}
                selected={{ section, index }}
                catalog={catalog}
                nameOf={nameOf}
                onAdd={addLink}
                onSelect={selectLink}
              />
            ) : (
              namedList(tab.id)
            )}
            {tab.id === 'services' && (
              <>
                {componentList(false)}
                {componentList(true)}
              </>
            )}
          </div>
          <RecordEditor
            draft={draft}
            section={section}
            index={index}
            catalog={catalog}
            issues={editor.issues}
            onChange={editor.edit}
            onRemove={remove}
          />
        </div>
        {owner && (
          <div className="cp-org__lists">
            <LinkTables
              draft={draft}
              selected={{ section, index }}
              catalog={catalog}
              nameOf={nameOf}
              owner={owner}
              onAdd={(kind) => {
                addLink(kind, owner)
              }}
              onSelect={selectLink}
            />
          </div>
        )}
        <div className="cp-org__bar cp-org__actions">
          <Button
            ref={opener}
            type="submit"
            view="action"
            disabled={!editor.dirty}
          >
            Просмотреть изменения
          </Button>
          <Button
            disabled={!editor.dirty}
            onClick={() => {
              setDiscardRequested(true)
            }}
          >
            Отмена
          </Button>
        </div>
      </form>
      {editor.proposal && (
        <ChangeConfirmationDialog
          title="Подтвердить структуру организации"
          scope="Структура организации"
          consequences="Команды, проекты и подтверждённые компоненты задают срезы отчётов. Связи определяют области доступа."
          proposal={editor.proposal}
          wide
          openerRef={opener}
          submit={editor.submit}
          recheck={editor.recheck}
          onSaved={editor.accept}
          onValidation={editor.onValidation}
          onRejected={(error) => {
            if (error.kind !== 'forbidden') return false
            void navigate(routePaths.unavailable, { replace: true })
            return true
          }}
          onClose={editor.closeConfirmation}
        />
      )}
      <UnsavedChangesDialog
        formName="Структура организации"
        formRef={formRef}
        dirty={editor.dirty}
        confirmationOpen={Boolean(editor.proposal)}
        onDiscard={discard}
        allowSearchChanges
        discardRequested={discardRequested}
        onKeepEditing={() => {
          setDiscardRequested(false)
        }}
      />
    </>
  )
}
