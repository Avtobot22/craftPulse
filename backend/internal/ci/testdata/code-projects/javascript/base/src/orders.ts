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
