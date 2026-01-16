from __future__ import annotations

import pytest

from dynamorm_py.query import decode_cursor, encode_cursor


def test_cursor_round_trip_with_bytes_and_nested_structures() -> None:
    key = {
        "PK": {"S": "A"},
        "SK": {"S": "B"},
        "blob": {"B": b"hi"},
        "nested": {"M": {"x": {"B": b"bye"}}},
        "list": {"L": [{"B": b"x"}, {"S": "y"}]},
    }
    cursor = encode_cursor(key, index="gsi-email", sort="ASC")
    decoded = decode_cursor(cursor)
    assert decoded.last_key == key
    assert decoded.index == "gsi-email"
    assert decoded.sort == "ASC"


def test_decode_cursor_invalid_base64_raises() -> None:
    with pytest.raises(ValueError):
        decode_cursor("bm90LWpzb24")  # base64url("not-json")
