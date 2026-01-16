from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import fields, is_dataclass
from decimal import Decimal
from typing import Any, cast, get_args, get_origin

import boto3
from boto3.dynamodb.types import TypeDeserializer, TypeSerializer
from botocore.exceptions import ClientError

from .errors import (
    AwsError,
    ConditionFailedError,
    EncryptionNotConfiguredError,
    NotFoundError,
    ValidationError,
)
from .model import AttributeDefinition, ModelDefinition


def _is_empty(value: Any) -> bool:
    if value is None:
        return True
    if value is False:
        return True
    if value == 0:
        return True
    if isinstance(value, (str, bytes, bytearray)) and len(value) == 0:
        return True
    if isinstance(value, (list, dict, set, tuple)) and len(value) == 0:
        return True
    return False


def _coerce_value(value: Any, annotation: Any) -> Any:
    if value is None:
        return None

    if annotation is int and isinstance(value, Decimal):
        return int(value)
    if annotation is float and isinstance(value, Decimal):
        return float(value)

    origin = get_origin(annotation)
    if origin is set and isinstance(value, set):
        (elem_type,) = get_args(annotation) or (Any,)
        return {_coerce_value(v, elem_type) for v in value}

    return value


def _map_client_error(err: ClientError) -> Exception:
    code = str(err.response.get("Error", {}).get("Code", ""))
    message = str(err.response.get("Error", {}).get("Message", ""))

    if code == "ConditionalCheckFailedException":
        return ConditionFailedError(message)
    if code == "ValidationException":
        return ValidationError(message)
    if code == "ResourceNotFoundException":
        return NotFoundError(message)

    return AwsError(code=code or "UnknownError", message=message or str(err))


class Table[T]:
    def __init__(
        self, model: ModelDefinition[T], *, client: Any | None = None, table_name: str | None = None
    ) -> None:
        if table_name is None:
            table_name = model.table_name
        if not table_name:
            raise ValueError("table_name is required (or set ModelDefinition.table_name)")

        self._model = model
        self._table_name = table_name
        self._client = client or boto3.client("dynamodb")
        self._serializer = TypeSerializer()
        self._deserializer = TypeDeserializer()

    def put(
        self,
        item: T,
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
    ) -> None:
        try:
            dynamodb_item = self._to_item(item)
            req: dict[str, Any] = {"TableName": self._table_name, "Item": dynamodb_item}
            if condition_expression:
                req["ConditionExpression"] = condition_expression
            if expression_attribute_names:
                req["ExpressionAttributeNames"] = dict(expression_attribute_names)
            if expression_attribute_values:
                req["ExpressionAttributeValues"] = self._serialize_values(expression_attribute_values)
            self._client.put_item(**req)
        except ClientError as err:  # pragma: no cover (depends on AWS error shapes)
            raise _map_client_error(err) from err

    def get(self, pk: Any, sk: Any | None = None, *, consistent_read: bool = False) -> T:
        key = self._to_key(pk, sk)
        try:
            resp = self._client.get_item(TableName=self._table_name, Key=key, ConsistentRead=consistent_read)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        item = resp.get("Item")
        if not item:
            raise NotFoundError("item not found")
        return self._from_item(item)

    def delete(
        self,
        pk: Any,
        sk: Any | None = None,
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
    ) -> None:
        key = self._to_key(pk, sk)
        req: dict[str, Any] = {"TableName": self._table_name, "Key": key}
        if condition_expression:
            req["ConditionExpression"] = condition_expression
        if expression_attribute_names:
            req["ExpressionAttributeNames"] = dict(expression_attribute_names)
        if expression_attribute_values:
            req["ExpressionAttributeValues"] = self._serialize_values(expression_attribute_values)

        try:
            self._client.delete_item(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

    def update(
        self,
        pk: Any,
        sk: Any | None,
        updates: Mapping[str, Any],
        *,
        condition_expression: str | None = None,
        expression_attribute_names: Mapping[str, str] | None = None,
        expression_attribute_values: Mapping[str, Any] | None = None,
    ) -> T:
        key = self._to_key(pk, sk)

        update_names: dict[str, str] = {}
        update_values: dict[str, Any] = {}
        set_parts: list[str] = []
        remove_parts: list[str] = []

        for field_name, value in updates.items():
            if field_name not in self._model.attributes:
                raise ValidationError(f"unknown field: {field_name}")
            if field_name == self._model.pk.python_name or (
                self._model.sk is not None and field_name == self._model.sk.python_name
            ):
                raise ValidationError(f"cannot update key field: {field_name}")

            attr_def = self._model.attributes[field_name]
            name_ref = f"#d_{field_name}"
            update_names[name_ref] = attr_def.attribute_name

            if value is None:
                remove_parts.append(name_ref)
                continue

            value_ref = f":d_{field_name}"
            update_values[value_ref] = self._serialize_attr_value(attr_def, value)
            set_parts.append(f"{name_ref} = {value_ref}")

        expr_parts: list[str] = []
        if set_parts:
            expr_parts.append("SET " + ", ".join(set_parts))
        if remove_parts:
            expr_parts.append("REMOVE " + ", ".join(remove_parts))
        if not expr_parts:
            raise ValidationError("no updates provided")

        req: dict[str, Any] = {
            "TableName": self._table_name,
            "Key": key,
            "UpdateExpression": " ".join(expr_parts),
            "ExpressionAttributeNames": update_names,
            "ReturnValues": "ALL_NEW",
        }
        if update_values:
            req["ExpressionAttributeValues"] = update_values

        if condition_expression:
            req["ConditionExpression"] = condition_expression

        if expression_attribute_names:
            for k, v in expression_attribute_names.items():
                if k in req["ExpressionAttributeNames"]:
                    raise ValidationError(f"expression attribute name collision: {k}")
                req["ExpressionAttributeNames"][k] = v

        if expression_attribute_values:
            req.setdefault("ExpressionAttributeValues", {})
            serialized = self._serialize_values(expression_attribute_values)
            for k, v in serialized.items():
                if k in req["ExpressionAttributeValues"]:
                    raise ValidationError(f"expression attribute value collision: {k}")
                req["ExpressionAttributeValues"][k] = v

        try:
            resp = self._client.update_item(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        attrs = resp.get("Attributes")
        if not attrs:
            raise ValidationError("update did not return Attributes")
        return self._from_item(attrs)

    def _serialize_values(self, values: Mapping[str, Any]) -> dict[str, Any]:
        out: dict[str, Any] = {}
        for k, v in values.items():
            out[k] = self._serializer.serialize(v)
        return out

    def _serialize_attr_value(self, attr_def: AttributeDefinition, value: Any) -> Any:
        if attr_def.encrypted:
            raise EncryptionNotConfiguredError(f"encrypted field requires encryption: {attr_def.python_name}")

        if attr_def.json and value is not None:
            value = json.dumps(value, separators=(",", ":"), sort_keys=True)

        return self._serializer.serialize(value)

    def _to_item(self, item: T) -> dict[str, Any]:
        if not is_dataclass(item):
            raise ValidationError("item must be a dataclass instance")

        out: dict[str, Any] = {}
        for field_name, attr_def in self._model.attributes.items():
            value = getattr(item, field_name)
            if attr_def.omitempty and _is_empty(value):
                continue
            out[attr_def.attribute_name] = self._serialize_attr_value(attr_def, value)

        if self._model.pk.attribute_name not in out:
            raise ValidationError("missing pk")
        if self._model.sk is not None and self._model.sk.attribute_name not in out:
            raise ValidationError("missing sk")

        return out

    def _to_key(self, pk: Any, sk: Any | None) -> dict[str, Any]:
        if pk is None:
            raise ValidationError("pk is required")
        if self._model.sk is None and sk is not None:
            raise ValidationError("model does not define sk")
        if self._model.sk is not None and sk is None:
            raise ValidationError("sk is required")

        key: dict[str, Any] = {self._model.pk.attribute_name: self._serializer.serialize(pk)}
        if self._model.sk is not None:
            key[self._model.sk.attribute_name] = self._serializer.serialize(sk)
        return key

    def _from_item(self, item: Mapping[str, Any]) -> T:
        model_cls = self._model.model_type
        model_annotations = getattr(model_cls, "__annotations__", {})

        kwargs: dict[str, Any] = {}
        for dc_field in fields(cast(Any, model_cls)):
            if dc_field.name not in self._model.attributes:
                continue

            attr_def = self._model.attributes[dc_field.name]
            if attr_def.attribute_name not in item:
                continue

            raw = self._deserializer.deserialize(item[attr_def.attribute_name])
            if attr_def.json and isinstance(raw, str):
                raw = json.loads(raw)

            kwargs[dc_field.name] = _coerce_value(raw, model_annotations.get(dc_field.name, Any))

        try:
            return model_cls(**kwargs)
        except TypeError as err:
            raise ValidationError(str(err)) from err
