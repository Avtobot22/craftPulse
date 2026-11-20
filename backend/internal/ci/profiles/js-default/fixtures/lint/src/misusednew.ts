export interface Factory {
  new (): Factory;
}

export interface Builder {
  build(): Builder;
}
