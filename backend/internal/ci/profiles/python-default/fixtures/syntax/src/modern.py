import sys

type Pair[T] = tuple[T, T]


def describe(value: object) -> str:
    match value:
        case [first, *rest]:
            return f"{first} and {len(rest)} more"
        case {"name": name}:
            return f"named {name}"
        case _:
            return "other"
