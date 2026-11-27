import { Button } from '@gravity-ui/uikit'
import type { Choice } from '../api/structure'
import { editRow, sections } from '../model/structure-draft'
import type { Draft, Section } from '../model/structure-draft'
import type { Issue } from '../model/structure-errors'
import type { Catalog } from './link-tables'
import {
  ComponentFields,
  fieldId,
  LinkFields,
  NamedFields,
} from './structure-fields'
import { RowErrors } from './structure-errors'

function choices(rows: Draft['teams']): Choice[] {
  return rows.flatMap(({ id, name }) => (id === null ? [] : [{ id, name }]))
}

export function RecordEditor({
  draft,
  section,
  index,
  issues,
  catalog,
  onChange,
  onRemove,
}: {
  draft: Draft
  section: Section
  index: number
  issues: Issue[]
  catalog: Catalog
  onChange: (draft: Draft) => void
  onRemove: () => void
}) {
  const errors = issues.filter(
    (issue) =>
      issue.section === section &&
      (issue.index === index || issue.index === undefined),
  )
  const row = draft[section][index]
  const teams = choices(draft.teams)
  const projects = choices(draft.projects)
  const services = choices(draft.services)
  const repositories = catalog.areas.filter(({ kind }) => kind === 'repository')
  const areaProblem =
    catalog.areaProblem ??
    (!catalog.areas.length ? 'Источник не настроен' : undefined)
  const repositoriesProblem =
    areaProblem ??
    (!repositories.length ? 'Подключите источник репозиториев' : undefined)
  const peopleProblem =
    catalog.peopleProblem ??
    (!catalog.people.length ? 'В организации пока нет участников' : undefined)
  if (!row)
    return (
      <section className="cp-org__editor" id={fieldId(section)} tabIndex={-1}>
        <h2>Редактирование</h2>
        <p>Выберите запись в списке или добавьте новую.</p>
        <RowErrors issues={errors} />
      </section>
    )

  return (
    <section
      className="cp-org__editor"
      id={fieldId(section)}
      tabIndex={-1}
      aria-label={sections[section] + ' · редактирование'}
    >
      <h2>{sections[section]} · редактирование</h2>
      <RowErrors issues={errors} />
      {'name' in row &&
        (section === 'teams' ||
          section === 'projects' ||
          section === 'services') && (
          <NamedFields
            section={section}
            row={row}
            issues={errors}
            onChange={(next) => {
              onChange(editRow(draft, section, index, next))
            }}
          />
        )}
      {'path_prefix' in row && (
        <ComponentFields
          row={row}
          issues={errors}
          services={services}
          repositories={repositories}
          unavailable={repositoriesProblem}
          onChange={(next) => {
            onChange(editRow(draft, 'components', index, next))
          }}
        />
      )}
      {'person_id' in row && (
        <LinkFields
          section="team_members"
          issues={errors}
          fields={[
            {
              field: 'team_id',
              value: row.team_id,
              choices: teams,
              onChange: (team_id) => {
                onChange(
                  editRow(draft, 'team_members', index, { ...row, team_id }),
                )
              },
            },
            {
              field: 'person_id',
              value: row.person_id,
              choices: catalog.people,
              unavailable: peopleProblem,
              onChange: (person_id) => {
                onChange(
                  editRow(draft, 'team_members', index, { ...row, person_id }),
                )
              },
            },
          ]}
        />
      )}
      {'area_id' in row && (
        <LinkFields
          section="project_areas"
          issues={errors}
          fields={[
            {
              field: 'project_id',
              value: row.project_id,
              choices: projects,
              onChange: (project_id) => {
                onChange(
                  editRow(draft, 'project_areas', index, {
                    ...row,
                    project_id,
                  }),
                )
              },
            },
            {
              field: 'area_id',
              value: row.area_id,
              choices: catalog.areas,
              unavailable: areaProblem,
              onChange: (area_id) => {
                onChange(
                  editRow(draft, 'project_areas', index, { ...row, area_id }),
                )
              },
            },
          ]}
        />
      )}
      {'team_id' in row && 'project_id' in row && (
        <LinkFields
          section="team_projects"
          issues={errors}
          fields={[
            {
              field: 'team_id',
              value: row.team_id,
              choices: teams,
              onChange: (team_id) => {
                onChange(
                  editRow(draft, 'team_projects', index, { ...row, team_id }),
                )
              },
            },
            {
              field: 'project_id',
              value: row.project_id,
              choices: projects,
              onChange: (project_id) => {
                onChange(
                  editRow(draft, 'team_projects', index, {
                    ...row,
                    project_id,
                  }),
                )
              },
            },
          ]}
        />
      )}
      {(!('id' in row) || row.id === null) && (
        <Button onClick={onRemove}>
          {'id' in row ? 'Убрать из черновика' : 'Снять связь'}
        </Button>
      )}
    </section>
  )
}
