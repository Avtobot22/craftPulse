import { orderTotal } from "./orders";

export function testOrderTotal(): boolean {
  return orderTotal([{ price: 2.5, quantity: 2 }]) === 5;
}
