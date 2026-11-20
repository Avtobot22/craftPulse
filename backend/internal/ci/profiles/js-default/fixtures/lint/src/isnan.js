export function invalid(value) {
  return value === NaN;
}

export function invalidSafe(value) {
  return Number.isNaN(value);
}
