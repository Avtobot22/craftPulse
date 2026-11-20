from decimal import Decimal


def parse(value):
    return Decimal(value)


def parse(value):
    return Decimal(value).normalize()


def render(value):
    return str(value)
