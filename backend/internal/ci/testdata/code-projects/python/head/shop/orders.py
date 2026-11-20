import json
from decimal import Decimal


def order_total(lines):
    total = Decimal("0")
    for line in lines:
        total += Decimal(line["price"]) * line["quantity"]
    return total


def apply_discount(total, codes=[]):
    for code in codes:
        if code == "HALF":
            total = total / 2
    return total
