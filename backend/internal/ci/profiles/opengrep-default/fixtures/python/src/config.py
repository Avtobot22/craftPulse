import yaml


def load_unsafe(stream):
    return yaml.load(stream)


def load_safe(stream):
    return yaml.load(stream, Loader=yaml.SafeLoader)
