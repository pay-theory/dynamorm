import {
  ConditionalCheckFailedException,
  TransactionCanceledException,
} from '@aws-sdk/client-dynamodb';

import { DynamormError } from './errors.js';

export function mapDynamoError(err: unknown): unknown {
  if (err instanceof DynamormError) return err;

  if (err instanceof ConditionalCheckFailedException) {
    return new DynamormError('ErrConditionFailed', 'Condition failed', {
      cause: err,
    });
  }

  if (err instanceof TransactionCanceledException) {
    return new DynamormError('ErrConditionFailed', 'Transaction canceled', {
      cause: err,
    });
  }

  if (typeof err === 'object' && err !== null && 'name' in err) {
    const name = (err as { name?: unknown }).name;
    if (name === 'ConditionalCheckFailedException') {
      return new DynamormError('ErrConditionFailed', 'Condition failed', {
        cause: err,
      });
    }
    if (name === 'TransactionCanceledException') {
      return new DynamormError('ErrConditionFailed', 'Transaction canceled', {
        cause: err,
      });
    }
  }

  return err;
}
