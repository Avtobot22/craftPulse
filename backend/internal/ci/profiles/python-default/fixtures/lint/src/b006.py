def append_to(item, target=[]):
    target.append(item)
    return target


def append_safe(item, target=None):
    if target is None:
        target = []
    target.append(item)
    return target
