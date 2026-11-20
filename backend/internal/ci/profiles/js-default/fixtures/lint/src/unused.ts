import type { Readable } from "node:stream";

export function size(items: string[]): number {
  const unused: number = items.length;
  return items.length;
}

export function sizeSafe(stream: Readable): boolean {
  return stream.readable;
}
