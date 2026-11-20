def over_limit_orders(items, limit):
    total = 0
    skipped = 0
    for item in items:
        amount = item.get("amount", 0)
        if amount > limit:
            total += amount * 2
        elif amount > 0:
            total += amount
        else:
            skipped += 1
    names = sorted(item["name"] for item in items if "name" in item)
    for name in names:
        print("item", name, total, skipped)
    report = {
        "total": total,
        "skipped": skipped,
        "limit": limit,
        "count": len(items),
    }
    if report["count"] == 0:
        report["average"] = 0
    else:
        report["average"] = total / report["count"]
    return report
