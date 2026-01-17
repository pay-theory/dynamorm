from __future__ import annotations

from dataclasses import dataclass

import pytest

from dynamorm_py import ModelDefinition, Table, ValidationError, dynamorm_field


@dataclass(frozen=True)
class Thing:
    pk: str = dynamorm_field(roles=["pk"])
    sk: str = dynamorm_field(roles=["sk"])
    value: int = dynamorm_field(default=0)


def test_query_invalid_cursor_raises_validation_error() -> None:
    model = ModelDefinition.from_dataclass(Thing, table_name="tbl")
    table: Table[Thing] = Table(model, client=object())

    with pytest.raises(ValidationError):
        table.query("A", cursor="not-a-valid-cursor")


def test_scan_invalid_cursor_raises_validation_error() -> None:
    model = ModelDefinition.from_dataclass(Thing, table_name="tbl")
    table: Table[Thing] = Table(model, client=object())

    with pytest.raises(ValidationError):
        table.scan(cursor="not-a-valid-cursor")
