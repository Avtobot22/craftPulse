import type { Item } from "./item";

export function overLimitInvoices(items: Item[], limit: number) {
  let total = 0;
  let skipped = 0;
  for (const item of items) {
    const amount = item.amount ?? 0;
    if (amount > limit) {
      total += amount * 2;
    } else if (amount > 0) {
      total += amount;
    } else {
      skipped += 1;
    }
  }
  const names = items.filter((item) => item.name).map((item) => item.name).sort();
  for (const name of names) {
    console.log("item", name, total, skipped);
  }
  const report = { total, skipped, limit, count: items.length, average: 0 };
  if (report.count > 0) {
    report.average = total / report.count;
  }
  return report;
}
