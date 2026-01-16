from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Any

import boto3

from dynamorm_py import ModelDefinition, Table, dynamorm_field


@dataclass(frozen=True)
class DemoItem:
    pk: str = dynamorm_field(name="PK", roles=["pk"])
    sk: str = dynamorm_field(name="SK", roles=["sk"])
    value: str = dynamorm_field(name="value", omitempty=True, default="")
    lang: str = dynamorm_field(name="lang", omitempty=True, default="")


_table: Table[DemoItem] | None = None


def _get_table() -> Table[DemoItem]:
    global _table
    if _table is not None:
        return _table

    table_name = (os.environ.get("TABLE_NAME") or "").strip()
    if not table_name:
        raise RuntimeError("TABLE_NAME is required")

    model = ModelDefinition.from_dataclass(DemoItem, table_name=table_name)
    client = boto3.client("dynamodb")
    _table = Table(model, client=client)
    return _table


def _json_response(status_code: int, body: Any) -> dict[str, Any]:
    return {
        "statusCode": status_code,
        "headers": {"content-type": "application/json"},
        "body": json.dumps(body, separators=(",", ":"), sort_keys=True),
    }


def handler(event: dict[str, Any], context: Any) -> dict[str, Any]:
    _ = context
    method = ((event.get("requestContext") or {}).get("http") or {}).get("method") or "GET"
    qs = event.get("queryStringParameters") or {}

    body_raw = event.get("body") or ""
    try:
        body = json.loads(body_raw) if body_raw else {}
    except Exception:
        body = {}

    pk = str(body.get("pk") or qs.get("pk") or "")
    sk = str(body.get("sk") or qs.get("sk") or "")
    value = str(body.get("value") or qs.get("value") or "")

    if not pk or not sk:
        return _json_response(400, {"error": "pk and sk are required"})

    table = _get_table()

    if method == "GET":
        item = table.get(pk, sk)
        return _json_response(200, {"ok": True, "item": item.__dict__})

    table.put(DemoItem(pk=pk, sk=sk, value=value, lang="py"))
    item = table.get(pk, sk)
    return _json_response(200, {"ok": True, "item": item.__dict__})

