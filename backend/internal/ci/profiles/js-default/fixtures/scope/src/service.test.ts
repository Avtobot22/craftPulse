import { remove } from "./service";

export function testRemove(): boolean {
  const unused = 2;
  return remove("a") === "a";
}
