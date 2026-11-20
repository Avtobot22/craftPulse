import sys

from service import remove


def test_remove():
    assert remove("a") == "a"
