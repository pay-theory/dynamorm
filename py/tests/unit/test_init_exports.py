from __future__ import annotations

import dynamorm_py as dynamorm


def test_init_exposes_lazy_exports_via_getattr() -> None:
    assert dynamorm._normalize_repo_version("1.2.3") == "1.2.3"

    assert callable(dynamorm.ensure_table)
    assert callable(dynamorm.aggregate_field)
    assert callable(dynamorm.QueryOptimizer)
    assert callable(dynamorm.is_lambda_environment)
    assert callable(dynamorm.MultiAccountSessions)
    assert callable(dynamorm.SimpleLimiter)
    assert callable(dynamorm.validate_expression)
