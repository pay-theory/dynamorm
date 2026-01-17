from __future__ import annotations

import json
from pathlib import Path

import dynamorm_py


def test_version_matches_version_json() -> None:
    version_file = Path(__file__).resolve().parents[2] / "src" / "dynamorm_py" / "version.json"
    data = json.loads(version_file.read_text(encoding="utf-8"))
    assert dynamorm_py.__repo_version__ == data["version"]
    if "-rc." in data["version"]:
        assert "-rc." not in dynamorm_py.__version__
        assert "rc" in dynamorm_py.__version__
    else:
        assert dynamorm_py.__version__ == data["version"]
