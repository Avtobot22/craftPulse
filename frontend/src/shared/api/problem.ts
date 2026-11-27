import type { components } from './generated/schema'
import {
  InvalidResponse,
  readArray,
  readEnum,
  readRecord,
  readString,
} from './response'

export type ApiProblem = components['schemas']['Problem']
export type FieldError = Omit<
  components['schemas']['FieldError'],
  'code' | 'detail'
> & {
  code: components['schemas']['FieldError']['code'] | 'unknown'
}
export type SafeProblem = Omit<ApiProblem, 'detail' | 'errors'> & {
  errors?: FieldError[]
}
export type ApiErrorKind =
  | 'auth-required'
  | 'forbidden'
  | 'not-found'
  | 'version-conflict'
  | 'validation'
  | 'precondition-required'
  | 'api-error'
  | 'network'
  | 'invalid-response'
  | 'missing-version'
  | 'session-ended'

export class ApiError extends Error {
  readonly kind: ApiErrorKind
  readonly problem: SafeProblem | null
  readonly status: number | undefined

  constructor(
    kind: ApiErrorKind,
    problem: SafeProblem | null = null,
    status = problem?.status,
  ) {
    super('Не удалось выполнить запрос')
    this.name = 'ApiError'
    this.kind = kind
    this.problem = problem
    this.status = status
  }
}

function readFieldError(value: unknown): FieldError {
  const raw = readRecord(value)
  return {
    field: readString(raw.field),
    code: readEnum<components['schemas']['FieldError']['code']>(raw.code, [
      'required',
      'invalid_format',
      'out_of_range',
      'too_long',
      'unknown_field',
      'not_allowed_value',
      'duplicate',
      'outside_scope',
      'unknown_reference',
    ]),
  }
}

export function readProblem(value: unknown, status: number): SafeProblem {
  const raw = readRecord(value)
  const correlation = readString(raw.correlation_id)
  if (
    raw.status !== status ||
    !Number.isInteger(status) ||
    status < 400 ||
    status > 599 ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/iu.test(
      correlation,
    )
  ) {
    throw new InvalidResponse()
  }
  return {
    type: readString(raw.type),
    title: readString(raw.title),
    status,
    code: readString(raw.code),
    correlation_id: correlation,
    ...(raw.errors === undefined
      ? {}
      : { errors: readArray(raw.errors, readFieldError) }),
  }
}

export function classifyProblem(problem: SafeProblem): ApiError {
  switch (problem.code) {
    case 'auth_required':
      return new ApiError('auth-required', problem)
    case 'forbidden':
      return new ApiError('forbidden', problem)
    case 'not_found':
      return new ApiError('not-found', problem)
    case 'version_conflict':
      return new ApiError('version-conflict', problem)
    case 'validation_failed':
      return new ApiError('validation', problem)
    case 'precondition_required':
      return new ApiError('precondition-required', problem)
    default:
      return new ApiError('api-error', problem)
  }
}

export function fieldErrorsByPointer(
  problem: SafeProblem,
): ReadonlyMap<string, readonly FieldError[]> {
  const fields = new Map<string, FieldError[]>()
  for (const error of problem.errors ?? []) {
    fields.set(error.field, [...(fields.get(error.field) ?? []), error])
  }
  return fields
}

export function isUnavailableError(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    (error.kind === 'forbidden' || error.kind === 'not-found')
  )
}
