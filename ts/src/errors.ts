export type ErrorCode =
  | 'ErrItemNotFound'
  | 'ErrConditionFailed'
  | 'ErrInvalidModel'
  | 'ErrMissingPrimaryKey'
  | 'ErrInvalidOperator'
  | 'ErrEncryptedFieldNotQueryable'
  | 'ErrEncryptionNotConfigured'
  | 'ErrInvalidEncryptedEnvelope';

export class DynamormError extends Error {
  readonly code: ErrorCode;

  constructor(code: ErrorCode, message: string, options?: { cause?: unknown }) {
    super(message);
    this.code = code;
    this.name = code;
    if (options?.cause !== undefined) {
      // Avoid depending on a specific TS libdom ErrorOptions typing.
      (this as { cause?: unknown }).cause = options.cause;
    }
  }
}

export function isDynamormError(value: unknown): value is DynamormError {
  return value instanceof DynamormError;
}
