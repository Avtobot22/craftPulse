export class InvalidResponse extends Error {
  constructor() {
    super('Ответ сервера не соответствует контракту')
    this.name = 'InvalidResponse'
  }
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function readRecord(value: unknown): Record<string, unknown> {
  if (!isRecord(value)) throw new InvalidResponse()
  return value
}

export function readString(value: unknown): string {
  if (typeof value !== 'string') throw new InvalidResponse()
  return value
}

export function readNullableString(value: unknown): string | null {
  return value === null ? null : readString(value)
}

export function readBoolean(value: unknown): boolean {
  if (typeof value !== 'boolean') throw new InvalidResponse()
  return value
}

export function readArray<T>(value: unknown, read: (item: unknown) => T): T[] {
  if (!Array.isArray(value)) throw new InvalidResponse()
  return value.map((item: unknown) => read(item))
}

export function readEnum<T extends string>(
  value: unknown,
  known: readonly T[],
): T | 'unknown' {
  const text = readString(value)
  return known.find((item) => item === text) ?? 'unknown'
}
