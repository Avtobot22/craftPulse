from decimal import Decimal


def order_total(lines):
    total = Decimal("0")
    for line in lines:
        total += Decimal(line["price"]) * line["quantity"]
    return total
