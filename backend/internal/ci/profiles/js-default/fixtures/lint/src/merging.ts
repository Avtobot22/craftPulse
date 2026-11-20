export interface Account {
  id: string;
}

export class Account {
  constructor(public name: string) {}
}

export interface Owner {
  id: string;
}

export class Team implements Owner {
  constructor(public id: string) {}
}
