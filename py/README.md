# DynamORM (Python)

Python implementation of DynamORM for DynamoDB.

This package is developed in a multi-language monorepo alongside the Go and TypeScript implementations. GitHub releases
are the source of truth for versions (no PyPI publishing).

## Requirements

- Python `>=3.14`
- AWS credentials (or DynamoDB Local) for integration tests/examples

## Install (from this monorepo)

This repo does not publish to PyPI. Install from source:

```bash
# from the repo root
pip install -e ./py

# or with uv (recommended for development)
uv --directory py sync
```

You can also install from a Git ref/tag:

```bash
pip install "git+https://github.com/pay-theory/dynamorm.git@vX.Y.Z#subdirectory=py"
```

## Quickstart

```python
from dataclasses import dataclass
import os

import boto3

from dynamorm_py import ModelDefinition, Table, dynamorm_field


@dataclass(frozen=True)
class Note:
    pk: str = dynamorm_field(roles=["pk"])
    sk: str = dynamorm_field(roles=["sk"])
    value: int = dynamorm_field()


client = boto3.client(
    "dynamodb",
    endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
    region_name=os.environ.get("AWS_REGION", "us-east-1"),
    aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
    aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
)

model = ModelDefinition.from_dataclass(Note, table_name="notes")
table = Table(model, client=client)

table.put(Note(pk="A", sk="1", value=123))
note = table.get("A", "1")
```

## Query + pagination

```python
from dynamorm_py import SortKeyCondition

page1 = table.query("A", sort=SortKeyCondition.begins_with("1"), limit=25)
page2 = table.query("A", cursor=page1.next_cursor) if page1.next_cursor else None
```

## Batch + transactions

```python
from dynamorm_py import TransactUpdate

table.batch_write(puts=[Note(pk="A", sk="2", value=1)], deletes=[("A", "1")])

table.transact_write(
    [
        TransactUpdate(
            pk="A",
            sk="2",
            updates={"value": 2},
            condition_expression="#v = :expected",
            expression_attribute_names={"#v": "value"},
            expression_attribute_values={":expected": 1},
        )
    ]
)
```

## Streams (Lambda)

```python
from dynamorm_py import unmarshal_stream_record

def handler(event, context):
    for record in event.get("Records", []):
        note = unmarshal_stream_record(model, record, image="NewImage")
        if note is None:
            continue
        # process note...
```

## Encryption (`encrypted`)

Encrypted fields are envelope-encrypted using AES-256-GCM with a data key from AWS KMS and stored as a DynamoDB map.
If a model contains `encrypted` fields, `Table(...)` fails closed unless `kms_key_arn` is configured.

```python
@dataclass(frozen=True)
class SecretNote:
    pk: str = dynamorm_field(roles=["pk"])
    sk: str = dynamorm_field(roles=["sk"])
    secret: str = dynamorm_field(encrypted=True)

model = ModelDefinition.from_dataclass(SecretNote, table_name="notes")
table = Table(model, client=client, kms_key_arn=os.environ["KMS_KEY_ARN"])
```

## Examples

- Local DynamoDB: `py/examples/local_crud.py`
- DynamoDB Streams handler: `py/examples/lambda_stream_handler.py`

## Parity statement (Python)

Implemented milestones: `PY-0` through `PY-6` (tooling, schema, CRUD, query/scan, batch/tx, streams unmarshalling, encryption).

Not yet implemented: `PY-7` docs/examples are in-progress and parity may still diverge from Go/TS in edge-case behavior.
