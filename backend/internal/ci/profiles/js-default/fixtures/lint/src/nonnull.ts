export function first(items?: string[]): string {
  return items?.[0]!;
}

export function firstSafe(items?: string[]): string | undefined {
  return items?.[0];
}
