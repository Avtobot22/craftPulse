const dateTime =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/u

export function isDateTime(value: string) {
  return dateTime.test(value) && Number.isFinite(Date.parse(value))
}
