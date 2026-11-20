export function total(values) {
  return values.reduce((sum, v) => sum + v, initial);
}

export function totalSafe(values) {
  return values.reduce((sum, v) => sum + v, 0);
}
