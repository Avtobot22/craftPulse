def status_text(code):
    if code == 200:
        return "ok"
    if code == 201:
        return "created"
    if code == 204:
        return "no content"
    if code == 301:
        return "moved"
    if code == 304:
        return "not modified"
    if code == 400:
        return "bad request"
    if code == 401:
        return "unauthorized"
    if code == 403:
        return "forbidden"
    if code == 404:
        return "not found"
    if code == 409:
        return "conflict"
    if code == 500:
        return "server error"
    if code == 503 or code == 504:
        return "unavailable"
    return "unknown"
