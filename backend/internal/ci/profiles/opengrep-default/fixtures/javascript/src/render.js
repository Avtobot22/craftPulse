export function showUnsafe(element, comment) {
  element.innerHTML = "<p>" + comment + "</p>";
}

export function showSafe(element, comment) {
  element.textContent = comment;
}
