from __future__ import annotations

import base64
import json
from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True)
class SortKeyCondition:
    op: str
    values: tuple[Any, ...]

    @staticmethod
    def eq(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op="=", values=(value,))

    @staticmethod
    def lt(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op="<", values=(value,))

    @staticmethod
    def lte(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op="<=", values=(value,))

    @staticmethod
    def gt(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op=">", values=(value,))

    @staticmethod
    def gte(value: Any) -> SortKeyCondition:
        return SortKeyCondition(op=">=", values=(value,))

    @staticmethod
    def between(low: Any, high: Any) -> SortKeyCondition:
        return SortKeyCondition(op="between", values=(low, high))

    @staticmethod
    def begins_with(prefix: Any) -> SortKeyCondition:
        return SortKeyCondition(op="begins_with", values=(prefix,))


@dataclass(frozen=True)
class Page[T]:
    items: list[T]
    next_cursor: str | None


def _jsonify(obj: Any) -> Any:
    if isinstance(obj, bytes):
        return {"__type": "bytes", "b64": base64.b64encode(obj).decode("ascii")}
    if isinstance(obj, dict):
        return {k: _jsonify(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [_jsonify(v) for v in obj]
    return obj


def _dejsonify(obj: Any) -> Any:
    if isinstance(obj, dict) and obj.get("__type") == "bytes":
        return base64.b64decode(obj["b64"])
    if isinstance(obj, dict):
        return {k: _dejsonify(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [_dejsonify(v) for v in obj]
    return obj


def encode_cursor(exclusive_start_key: Any) -> str:
    payload = json.dumps(_jsonify(exclusive_start_key), separators=(",", ":"), sort_keys=True).encode("utf-8")
    return base64.urlsafe_b64encode(payload).decode("ascii").rstrip("=")


def decode_cursor(cursor: str) -> Any:
    padding = "=" * (-len(cursor) % 4)
    raw = base64.urlsafe_b64decode(cursor + padding).decode("utf-8")
    return _dejsonify(json.loads(raw))
