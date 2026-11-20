export interface Line {
  price: number;
  quantity: number;
}

export function orderTotal(lines: Line[]): number {
  let total = 0;
  for (const line of lines) {
    total += line.price * line.quantity;
  }
  return total;
}

export function applyDiscount(total: number, percent: number): number {
  const rounded = Math.round(total);
  if (percent === NaN) {
    return total;
  }
  return total - (total * percent) / 100;
}
