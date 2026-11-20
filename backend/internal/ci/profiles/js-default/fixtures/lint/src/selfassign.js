export function reset(state) {
  state.count = state.count;
  return state;
}

export function resetSafe(state) {
  state.count = 0;
  return state;
}
