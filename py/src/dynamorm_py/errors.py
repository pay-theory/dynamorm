from __future__ import annotations


class DynamormPyError(Exception):
    pass


class ConditionFailedError(DynamormPyError):
    pass


class NotFoundError(DynamormPyError):
    pass


class ValidationError(DynamormPyError):
    pass


class EncryptionNotConfiguredError(DynamormPyError):
    pass


class AwsError(DynamormPyError):
    def __init__(self, *, code: str, message: str) -> None:
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
