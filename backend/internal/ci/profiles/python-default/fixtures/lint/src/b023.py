def callbacks(names):
    handlers = []
    for name in names:
        handlers.append(lambda: print(name))
    return handlers


def callbacks_safe(names):
    handlers = []
    for name in names:
        handlers.append(lambda name=name: print(name))
    return handlers
