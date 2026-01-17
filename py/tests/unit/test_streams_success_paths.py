from __future__ import annotations

import base64
from dataclasses import dataclass
from typing import Any

import pytest

from dynamorm_py import ModelDefinition, ValidationError, dynamorm_field, unmarshal_stream_image
from dynamorm_py.model import AttributeDefinition
from dynamorm_py.streams import unmarshal_stream_record


@dataclass(frozen=True)
class StreamModel:
    pk: str = dynamorm_field(roles=["pk"])
    sk: str = dynamorm_field(roles=["sk"])
    count: int = dynamorm_field()
    rating: float = dynamorm_field()
    payload: dict[str, int] = dynamorm_field(json=True)
    tags: set[int] = dynamorm_field(set_=True, default_factory=set)
    labels: set[str] = dynamorm_field(set_=True, default_factory=set)
    blobs: set[bytes] = dynamorm_field(set_=True, default_factory=set)
    active: bool = dynamorm_field(default=False)
    maybe: str | None = dynamorm_field(default=None)
    meta: dict[str, Any] = dynamorm_field(default_factory=dict)
    nums: list[int] = dynamorm_field(default_factory=list)
    ignored: str = dynamorm_field(ignore=True, default="x")


def test_unmarshal_stream_image_decodes_types_and_coerces_numbers() -> None:
    model = ModelDefinition.from_dataclass(StreamModel)
    blob1 = base64.b64encode(b"a").decode("ascii")
    blob2 = base64.b64encode(b"b").decode("ascii")

    image = {
        "pk": {"S": "A"},
        "sk": {"S": "1"},
        "count": {"N": "5"},
        "rating": {"N": "1.5"},
        "tags": {"NS": ["1", "2"]},
        "labels": {"SS": ["x", "y"]},
        "payload": {"S": '{"a":1}'},
        "blobs": {"BS": [blob1, blob2]},
        "active": {"BOOL": True},
        "maybe": {"NULL": True},
        "meta": {"M": {"k": {"S": "v"}, "n": {"N": "2"}}},
        "nums": {"L": [{"N": "1"}, {"N": "2"}]},
    }

    got = unmarshal_stream_image(model, image)
    assert got.pk == "A"
    assert got.sk == "1"
    assert got.count == 5
    assert got.rating == 1.5
    assert got.tags == {1, 2}
    assert got.labels == {"x", "y"}
    assert got.payload == {"a": 1}
    assert got.active is True
    assert got.maybe is None
    assert got.meta["k"] == "v"
    assert b"a" in got.blobs and b"b" in got.blobs

    record = {"dynamodb": {"NewImage": image}}
    assert unmarshal_stream_record(model, record) == got


def test_unmarshal_stream_image_attribute_value_must_be_map() -> None:
    model = ModelDefinition.from_dataclass(StreamModel)
    with pytest.raises(ValidationError, match="attribute value must be a map"):
        unmarshal_stream_image(model, {"pk": "A"})


def test_unmarshal_stream_image_model_type_must_be_dataclass() -> None:
    pk = AttributeDefinition(
        python_name="pk",
        attribute_name="pk",
        roles=("pk",),
        omitempty=False,
        set=False,
        json=False,
        binary=False,
        encrypted=False,
    )
    model = ModelDefinition(
        model_type=int, table_name=None, pk=pk, sk=None, attributes={"pk": pk}, indexes=()
    )
    with pytest.raises(ValidationError, match="model_type must be a dataclass"):
        unmarshal_stream_image(model, {"pk": {"S": "A"}})


def test_unmarshal_stream_image_wraps_dataclass_constructor_errors() -> None:
    @dataclass(frozen=True)
    class RequiresField:
        pk: str = dynamorm_field(roles=["pk"])
        required: str = dynamorm_field()

    model = ModelDefinition.from_dataclass(RequiresField)
    with pytest.raises(ValidationError):
        unmarshal_stream_image(model, {"pk": {"S": "A"}})
