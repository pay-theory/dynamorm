from __future__ import annotations

from dataclasses import dataclass

from dynamorm_py import ModelDefinition, Table, dynamorm_field
from dynamorm_py.mocks import ANY, FakeDynamoDBClient, FakeKmsClient


@dataclass(frozen=True)
class Note:
    pk: str = dynamorm_field(roles=["pk"])
    sk: str = dynamorm_field(roles=["sk"])
    value: int = dynamorm_field()


def test_fake_dynamodb_client_records_and_matches_put_item() -> None:
    client = FakeDynamoDBClient()
    client.expect("put_item", {"TableName": "notes", "Item": ANY})

    model = ModelDefinition.from_dataclass(Note, table_name="notes")
    table = Table(model, client=client)

    table.put(Note(pk="A", sk="B", value=1))

    client.assert_no_pending()
    assert client.calls[0][0] == "put_item"


@dataclass(frozen=True)
class SecretNote:
    pk: str = dynamorm_field(roles=["pk"])
    sk: str = dynamorm_field(roles=["sk"])
    secret: str = dynamorm_field(encrypted=True)


def _as_bytes(value: object) -> bytes:
    if isinstance(value, (bytes, bytearray)):
        return bytes(value)
    return bytes(value)  # type: ignore[arg-type]


def test_table_encryption_can_be_deterministic_with_fake_clients() -> None:
    fixed_nonce = b"\x02" * 12
    plaintext_key = b"\x01" * 32
    edk = b"ciphertext-data-key"

    kms = FakeKmsClient(plaintext_key=plaintext_key, ciphertext_blob=edk)
    ddb = FakeDynamoDBClient()

    def validate_put(req: dict) -> None:
        item = req["Item"]
        env = item["secret"]["M"]

        assert env["v"]["N"] == "1"
        assert _as_bytes(env["edk"]["B"]) == edk
        assert _as_bytes(env["nonce"]["B"]) == fixed_nonce
        assert _as_bytes(env["ct"]["B"])

    ddb.expect("put_item", validate_put)

    model = ModelDefinition.from_dataclass(SecretNote, table_name="notes")
    table = Table(
        model,
        client=ddb,
        kms_key_arn="arn:aws:kms:us-east-1:111111111111:key/test",
        kms_client=kms,
        rand_bytes=lambda n: fixed_nonce[:n],
    )

    table.put(SecretNote(pk="A", sk="B", secret="top-secret"))

    ddb.assert_no_pending()
    assert [c[0] for c in kms.calls] == ["generate_data_key"]
