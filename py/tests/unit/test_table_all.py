from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any

from dynamorm_py.mocks import FakeDynamoDBClient
from dynamorm_py.model import ModelDefinition, dynamorm_field
from dynamorm_py.table import Table


@dataclass
class User:
    pk: str = dynamorm_field(name="PK", roles=["pk"])
    sk: str = dynamorm_field(name="SK", roles=["sk"])
    version: int = dynamorm_field(name="version")


def test_query_all_paginates_until_cursor_exhausted() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=client)

    last = {"PK": {"S": "A"}, "SK": {"S": "1"}}

    def first(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert "ExclusiveStartKey" not in req

    def second(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert req["ExclusiveStartKey"] == last

    client.expect(
        "query",
        expected=first,
        response={
            "Items": [{"PK": {"S": "A"}, "SK": {"S": "1"}, "version": {"N": "1"}}],
            "LastEvaluatedKey": last,
        },
    )
    client.expect(
        "query",
        expected=second,
        response={"Items": [{"PK": {"S": "A"}, "SK": {"S": "2"}, "version": {"N": "2"}}]},
    )

    items = table.query_all("A")
    assert [i.version for i in items] == [1, 2]
    client.assert_no_pending()


def test_scan_all_paginates_until_cursor_exhausted() -> None:
    client = FakeDynamoDBClient()
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=client)

    last = {"PK": {"S": "A"}, "SK": {"S": "1"}}

    def first(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert "ExclusiveStartKey" not in req

    def second(req: Mapping[str, Any]) -> None:
        assert req["TableName"] == "users"
        assert req["ExclusiveStartKey"] == last

    client.expect(
        "scan",
        expected=first,
        response={
            "Items": [{"PK": {"S": "A"}, "SK": {"S": "1"}, "version": {"N": "1"}}],
            "LastEvaluatedKey": last,
        },
    )
    client.expect(
        "scan",
        expected=second,
        response={"Items": [{"PK": {"S": "A"}, "SK": {"S": "2"}, "version": {"N": "2"}}]},
    )

    items = table.scan_all()
    assert [i.version for i in items] == [1, 2]
    client.assert_no_pending()
