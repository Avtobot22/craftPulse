from shop.orders import order_total


def test_order_total():
    assert order_total([{"price": "2.50", "quantity": 2}]) == 5
