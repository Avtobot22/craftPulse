export function matches(a, b) {
  if (a = b) {
    return true;
  }
  return false;
}

export function matchesSafe(a, b) {
  if (a === b) {
    return true;
  }
  return false;
}
