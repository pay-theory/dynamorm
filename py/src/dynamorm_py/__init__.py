from __future__ import annotations

import json
import re
from importlib.resources import files
from typing import TYPE_CHECKING, Any

from .errors import (
    AwsError,
    BatchRetryExceededError,
    ConditionFailedError,
    DynamormPyError,
    EncryptionNotConfiguredError,
    NotFoundError,
    TransactionCanceledError,
    ValidationError,
)
from .model import (
    IndexDefinition,
    IndexSpec,
    ModelDefinition,
    ModelDefinitionError,
    Projection,
    dynamorm_field,
    gsi,
    lsi,
)
from .query import Page, SortKeyCondition
from .transaction import (
    TransactConditionCheck,
    TransactDelete,
    TransactPut,
    TransactUpdate,
    TransactWriteAction,
)

if TYPE_CHECKING:
    from .dms import (
        assert_model_definition_equivalent_to_dms as assert_model_definition_equivalent_to_dms,
    )
    from .dms import get_dms_model as get_dms_model
    from .dms import parse_dms_document as parse_dms_document
    from .streams import unmarshal_stream_image as unmarshal_stream_image
    from .streams import unmarshal_stream_record as unmarshal_stream_record
    from .table import Table as Table


def _read_repo_version() -> str:
    try:
        data = json.loads(files(__package__).joinpath("version.json").read_text(encoding="utf-8"))
    except Exception:
        return "0.0.0"

    version = data.get("version")
    return version if isinstance(version, str) and version else "0.0.0"


def _normalize_repo_version(repo_version: str) -> str:
    match = re.match(r"^(\d+\.\d+\.\d+)-rc\.?([0-9]+)$", repo_version)
    if match:
        return f"{match.group(1)}rc{match.group(2)}"
    return repo_version


__repo_version__ = _read_repo_version()
__version__ = _normalize_repo_version(__repo_version__)


def __getattr__(name: str) -> Any:
    if name in {"parse_dms_document", "get_dms_model", "assert_model_definition_equivalent_to_dms"}:
        from . import dms

        return getattr(dms, name)
    if name == "Table":
        from .table import Table

        return Table
    if name == "unmarshal_stream_image":
        from .streams import unmarshal_stream_image

        return unmarshal_stream_image
    if name == "unmarshal_stream_record":
        from .streams import unmarshal_stream_record

        return unmarshal_stream_record
    raise AttributeError(name)


__all__ = [
    "AwsError",
    "assert_model_definition_equivalent_to_dms",
    "BatchRetryExceededError",
    "ConditionFailedError",
    "DynamormPyError",
    "EncryptionNotConfiguredError",
    "get_dms_model",
    "IndexDefinition",
    "IndexSpec",
    "ModelDefinition",
    "ModelDefinitionError",
    "NotFoundError",
    "Projection",
    "Page",
    "SortKeyCondition",
    "TransactConditionCheck",
    "TransactDelete",
    "TransactPut",
    "TransactUpdate",
    "TransactWriteAction",
    "TransactionCanceledError",
    "Table",
    "ValidationError",
    "__repo_version__",
    "__version__",
    "dynamorm_field",
    "gsi",
    "lsi",
    "parse_dms_document",
    "unmarshal_stream_image",
    "unmarshal_stream_record",
]
