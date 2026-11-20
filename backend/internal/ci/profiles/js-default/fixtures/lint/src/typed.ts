type Handler<T extends object> = (event: T) => Promise<void>;

interface Payment {
  readonly id: string;
  amount: number;
}

export const config = { retries: 3 } satisfies Record<string, number>;

export abstract class Queue<T extends object> {
  private readonly handlers: Handler<T>[] = [];

  on(handler: Handler<T>): this {
    this.handlers.push(handler);
    return this;
  }

  abstract name(): string;
}

export function rejected(payment: Payment): boolean {
  return payment.amount == NaN;
}
