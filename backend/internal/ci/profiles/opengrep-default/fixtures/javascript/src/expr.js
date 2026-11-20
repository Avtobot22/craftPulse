export function computeUnsafe(expression) {
  return eval(expression);
}

export function computeSafe(expression) {
  return Number.parseFloat(expression);
}
