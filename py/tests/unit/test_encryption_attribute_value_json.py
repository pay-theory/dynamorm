from __future__ import annotations

import base64

import pytest

from dynamorm_py.encryption import marshal_attribute_value_json, unmarshal_attribute_value_json
from dynamorm_py.errors import ValidationError


def test_marshal_attribute_value_json_supported_types() -> None:
    assert marshal_attribute_value_json({"S": "x"}) == {"Type": "S", "S": "x"}
    assert marshal_attribute_value_json({"N": "1"}) == {"Type": "N", "N": "1"}
    assert marshal_attribute_value_json({"BOOL": True}) == {"Type": "BOOL", "BOOL": True}
    assert marshal_attribute_value_json({"NULL": True}) == {"Type": "NULL", "NULL": True}
    assert marshal_attribute_value_json({"SS": ["a", "b"]}) == {"Type": "SS", "SS": ["a", "b"]}
    assert marshal_attribute_value_json({"NS": ["1", "2"]}) == {"Type": "NS", "NS": ["1", "2"]}

    assert marshal_attribute_value_json({"B": b"hi"}) == {
        "Type": "B",
        "B": base64.b64encode(b"hi").decode("ascii"),
    }
    assert marshal_attribute_value_json({"BS": [b"a", b"b"]}) == {
        "Type": "BS",
        "BS": [base64.b64encode(b"a").decode("ascii"), base64.b64encode(b"b").decode("ascii")],
    }

    assert marshal_attribute_value_json({"L": [{"S": "x"}, {"N": "1"}]}) == {
        "Type": "L",
        "L": [{"Type": "S", "S": "x"}, {"Type": "N", "N": "1"}],
    }
    assert marshal_attribute_value_json({"M": {"a": {"S": "x"}}}) == {
        "Type": "M",
        "M": {"a": {"Type": "S", "S": "x"}},
    }


def test_marshal_attribute_value_json_validation_errors() -> None:
    with pytest.raises(ValidationError):
        marshal_attribute_value_json("not-a-map")
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"S": "x", "N": "1"})

    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"S": 1})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"N": 1})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"B": "not-bytes"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"BOOL": "true"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"NULL": False})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"SS": ["a", 1]})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"NS": [1]})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"BS": ["not-bytes"]})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"L": "not-a-list"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"M": "not-a-map"})
    with pytest.raises(ValidationError):
        marshal_attribute_value_json({"X": "nope"})


def test_unmarshal_attribute_value_json_supported_types() -> None:
    assert unmarshal_attribute_value_json({"Type": "S", "S": "x"}) == {"S": "x"}
    assert unmarshal_attribute_value_json({"Type": "N", "N": "1"}) == {"N": "1"}
    assert unmarshal_attribute_value_json({"Type": "BOOL", "BOOL": False}) == {"BOOL": False}
    assert unmarshal_attribute_value_json({"Type": "NULL", "NULL": True}) == {"NULL": True}
    assert unmarshal_attribute_value_json({"Type": "SS", "SS": ["a"]}) == {"SS": ["a"]}
    assert unmarshal_attribute_value_json({"Type": "NS", "NS": ["1"]}) == {"NS": ["1"]}

    b64 = base64.b64encode(b"hi").decode("ascii")
    assert unmarshal_attribute_value_json({"Type": "B", "B": b64}) == {"B": b"hi"}
    assert unmarshal_attribute_value_json({"Type": "BS", "BS": [b64]}) == {"BS": [b"hi"]}

    assert unmarshal_attribute_value_json({"Type": "L", "L": [{"Type": "S", "S": "x"}]}) == {
        "L": [{"S": "x"}]
    }
    assert unmarshal_attribute_value_json({"Type": "M", "M": {"a": {"Type": "N", "N": "1"}}}) == {
        "M": {"a": {"N": "1"}}
    }


def test_unmarshal_attribute_value_json_validation_errors() -> None:
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json("not-a-map")
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"S": "x"})

    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "S", "S": 1})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "N", "N": 1})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "BOOL", "BOOL": "true"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "NULL", "NULL": False})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "SS", "SS": ["a", 1]})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "NS", "NS": [1]})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "B", "B": "not-base64"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "BS", "BS": ["not-base64"]})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "L", "L": "not-a-list"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "M", "M": "not-a-map"})
    with pytest.raises(ValidationError):
        unmarshal_attribute_value_json({"Type": "X"})
