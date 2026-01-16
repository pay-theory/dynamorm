from __future__ import annotations

import json
import re
from importlib.resources import files

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

__all__ = [
    "IndexDefinition",
    "IndexSpec",
    "ModelDefinition",
    "ModelDefinitionError",
    "Projection",
    "__repo_version__",
    "__version__",
    "dynamorm_field",
    "gsi",
    "lsi",
]
