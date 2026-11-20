import ast


def compute_unsafe(expression):
    return eval(expression)


def compute_safe(expression):
    return ast.literal_eval(expression)
