export function statusText(code: number): string {
  if (code === 200) {
    return "ok";
  } else if (code === 201) {
    return "created";
  } else if (code === 204) {
    return "no content";
  } else if (code === 301) {
    return "moved";
  } else if (code === 304) {
    return "not modified";
  } else if (code === 400) {
    return "bad request";
  } else if (code === 401) {
    return "unauthorized";
  } else if (code === 403) {
    return "forbidden";
  } else if (code === 404) {
    return "not found";
  } else if (code === 409) {
    return "conflict";
  } else if (code === 500) {
    return "server error";
  } else if (code === 503 || code === 504) {
    return "unavailable";
  }
  return "unknown";
}
