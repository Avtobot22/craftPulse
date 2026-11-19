import type { components, Team } from '@/shared/api'
import {
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
  readTeam,
} from '@/shared/api'

type Schema = components['schemas']
export type RoleAssignment = Omit<Schema['RoleAssignment'], 'role'> & {
  role: Schema['Role'] | 'unknown'
}
export type CurrentUser = Omit<
  Schema['CurrentUser'],
  'user' | 'organization' | 'roles' | 'areas'
> & {
  user: Omit<Schema['User'], 'status'> & {
    status: Schema['User']['status'] | 'unknown'
  }
  organization: Omit<Schema['CurrentUser']['organization'], 'status'> & {
    status: Schema['CurrentUser']['organization']['status'] | 'unknown'
  }
  roles: RoleAssignment[]
  areas: { teams: Team[]; projects: Team[]; services: Team[] }
}

function readRole(value: unknown): RoleAssignment {
  const raw = readRecord(value)
  return {
    role: readEnum<Schema['Role']>(raw.role, [
      'administrator',
      'dev_lead',
      'project_manager',
      'tech_lead',
      'developer',
      'process_owner',
      'service_owner',
      'security_access',
      'wiki_access',
    ]),
    team_id: readNullableString(raw.team_id),
    project_id: readNullableString(raw.project_id),
    service_id: readNullableString(raw.service_id),
    assigned_at: readString(raw.assigned_at),
  }
}

export function readCurrentUser(value: unknown): CurrentUser {
  const raw = readRecord(value)
  const user = readRecord(raw.user)
  const organization = readRecord(raw.organization)
  const areas = readRecord(raw.areas)
  const session = readRecord(raw.session)
  const participation =
    raw.review_participation === undefined
      ? undefined
      : readRecord(raw.review_participation)
  return {
    user: {
      id: readString(user.id),
      display_name: readNullableString(user.display_name),
      person_id: readNullableString(user.person_id),
      status: readEnum<Schema['User']['status']>(user.status, [
        'active',
        'blocked',
      ]),
    },
    organization: {
      id: readString(organization.id),
      name: readString(organization.name),
      slug: readString(organization.slug),
      status: readEnum<Schema['CurrentUser']['organization']['status']>(
        organization.status,
        ['setup', 'active', 'blocked', 'deleting'],
      ),
    },
    roles: readArray(raw.roles, readRole),
    actions: readArray(raw.actions, readString),
    areas: {
      teams: readArray(areas.teams, readTeam),
      projects: readArray(areas.projects, readTeam),
      services: readArray(areas.services, readTeam),
    },
    session: {
      expires_at: readString(session.expires_at),
      idle_expires_at: readString(session.idle_expires_at),
    },
    ...(participation === undefined
      ? {}
      : {
          review_participation: {
            participates: readBoolean(participation.participates),
          },
        }),
  }
}
