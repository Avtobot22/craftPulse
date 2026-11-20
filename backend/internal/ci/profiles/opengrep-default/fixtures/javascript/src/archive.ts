import { exec, execFile } from "child_process";

export function archiveUnsafe(path: string): void {
  exec(`tar czf backup.tgz ${path}`);
}

export function archiveSafe(path: string): void {
  execFile("tar", ["czf", "backup.tgz", path]);
}

export function firstDigits(text: string): string | null {
  const match = /\d+/.exec(text);
  return match ? match[0] : null;
}
