import {
  type AttributeValue,
  BatchGetItemCommand,
  BatchWriteItemCommand,
  ConditionalCheckFailedException,
  DeleteItemCommand,
  DynamoDBClient,
  GetItemCommand,
  PutItemCommand,
  TransactWriteItemsCommand,
  TransactionCanceledException,
  UpdateItemCommand,
  type ConditionCheck,
  type Delete,
  type Put,
  type TransactWriteItem,
  type WriteRequest,
} from '@aws-sdk/client-dynamodb';

import {
  chunk,
  sleep,
  type BatchGetResult,
  type BatchWriteResult,
  type RetryOptions,
} from './batch.js';
import { DynamormError } from './errors.js';
import type { Model } from './model.js';
import {
  isEmpty,
  marshalKey,
  marshalPutItem,
  marshalScalar,
  nowRfc3339Nano,
  unmarshalItem,
} from './marshal.js';
import { QueryBuilder, ScanBuilder } from './query.js';
import type { TransactAction } from './transaction.js';

export class DynamormClient {
  private readonly models = new Map<string, Model>();

  constructor(private readonly ddb: DynamoDBClient) {}

  register(...models: Model[]): this {
    for (const model of models) {
      this.models.set(model.name, model);
    }
    return this;
  }

  private requireModel(name: string): Model {
    const model = this.models.get(name);
    if (!model)
      throw new DynamormError('ErrInvalidModel', `Unknown model: ${name}`);
    return model;
  }

  async create(
    modelName: string,
    item: Record<string, unknown>,
    opts: { ifNotExists?: boolean } = {},
  ): Promise<void> {
    const model = this.requireModel(modelName);

    const now = nowRfc3339Nano();
    const putItem = marshalPutItem(model, item, { now });

    const cmd = new PutItemCommand({
      TableName: model.tableName,
      Item: putItem,
      ...(opts.ifNotExists
        ? {
            ConditionExpression: 'attribute_not_exists(#pk)',
            ExpressionAttributeNames: { '#pk': model.roles.pk },
          }
        : {}),
    });

    try {
      await this.ddb.send(cmd);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async get(
    modelName: string,
    key: Record<string, unknown>,
  ): Promise<Record<string, unknown>> {
    const model = this.requireModel(modelName);
    const cmd = new GetItemCommand({
      TableName: model.tableName,
      Key: marshalKey(model, key),
      ConsistentRead: true,
    });

    try {
      const resp = await this.ddb.send(cmd);
      if (!resp.Item)
        throw new DynamormError('ErrItemNotFound', 'Item not found');
      return unmarshalItem(model, resp.Item);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async update(
    modelName: string,
    item: Record<string, unknown>,
    fields: string[],
  ): Promise<void> {
    const model = this.requireModel(modelName);
    const key = marshalKey(model, item);

    const versionAttr = model.roles.version;
    if (!versionAttr)
      throw new DynamormError(
        'ErrInvalidModel',
        `Model ${model.name} does not define a version field`,
      );
    const currentVersion = item[versionAttr];
    if (
      currentVersion === undefined ||
      currentVersion === null ||
      currentVersion === ''
    ) {
      throw new DynamormError(
        'ErrInvalidModel',
        `Update requires current version in field: ${versionAttr}`,
      );
    }

    const now = nowRfc3339Nano();
    const names: Record<string, string> = {
      '#ver': versionAttr,
    };
    const values: Record<string, AttributeValue> = {
      ':expected': { N: String(currentVersion) },
      ':inc': { N: '1' },
    };

    const setParts: string[] = [];
    const removeParts: string[] = [];

    if (model.roles.updatedAt) {
      names['#updatedAt'] = model.roles.updatedAt;
      values[':now'] = { S: now };
      setParts.push('#updatedAt = :now');
    }

    for (const field of fields) {
      const fieldIndex = setParts.length + removeParts.length;
      if (field === model.roles.pk || field === model.roles.sk) {
        throw new DynamormError(
          'ErrInvalidModel',
          `Cannot update primary key field: ${field}`,
        );
      }
      if (field === model.roles.createdAt) {
        throw new DynamormError(
          'ErrInvalidModel',
          `Cannot update createdAt field: ${field}`,
        );
      }
      if (field === versionAttr) {
        throw new DynamormError(
          'ErrInvalidModel',
          `Do not include version in update fields: ${field}`,
        );
      }

      const schema = model.attributes.get(field);
      if (!schema)
        throw new DynamormError(
          'ErrInvalidModel',
          `Unknown field for model ${model.name}: ${field}`,
        );

      const value = item[field];
      if (value === undefined) {
        throw new DynamormError(
          'ErrInvalidModel',
          `Missing update value for field: ${field}`,
        );
      }

      const placeholder = `#f${fieldIndex}`;
      names[placeholder] = field;

      if (schema.omit_empty && isEmpty(value)) {
        removeParts.push(placeholder);
        continue;
      }

      const valueKey = `:v${fieldIndex}`;
      values[valueKey] = marshalScalar(schema, value);
      setParts.push(`${placeholder} = ${valueKey}`);
    }

    const updateParts: string[] = [];
    if (setParts.length) updateParts.push(`SET ${setParts.join(', ')}`);
    if (removeParts.length)
      updateParts.push(`REMOVE ${removeParts.join(', ')}`);
    updateParts.push(`ADD #ver :inc`);

    const cmd = new UpdateItemCommand({
      TableName: model.tableName,
      Key: key,
      ConditionExpression: '#ver = :expected',
      UpdateExpression: updateParts.join(' '),
      ExpressionAttributeNames: names,
      ExpressionAttributeValues: values,
    });

    try {
      await this.ddb.send(cmd);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async delete(modelName: string, key: Record<string, unknown>): Promise<void> {
    const model = this.requireModel(modelName);
    const cmd = new DeleteItemCommand({
      TableName: model.tableName,
      Key: marshalKey(model, key),
    });

    try {
      await this.ddb.send(cmd);
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  async batchGet(
    modelName: string,
    keys: Array<Record<string, unknown>>,
    opts: RetryOptions & { consistentRead?: boolean } = {},
  ): Promise<BatchGetResult> {
    const model = this.requireModel(modelName);

    const maxAttempts = opts.maxAttempts ?? 5;
    const baseDelayMs = opts.baseDelayMs ?? 25;
    const consistentRead = opts.consistentRead ?? true;

    const allItems: Array<Record<string, unknown>> = [];
    const unprocessedKeys: Array<Record<string, AttributeValue>> = [];

    for (const keyChunk of chunk(keys, 100)) {
      let pending = keyChunk.map((k) => marshalKey(model, k));

      for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        const resp = await this.ddb.send(
          new BatchGetItemCommand({
            RequestItems: {
              [model.tableName]: {
                Keys: pending,
                ConsistentRead: consistentRead,
              },
            },
          }),
        );

        const got = resp.Responses?.[model.tableName] ?? [];
        allItems.push(...got.map((it) => unmarshalItem(model, it)));

        const next = resp.UnprocessedKeys?.[model.tableName]?.Keys ?? [];
        if (next.length === 0) {
          pending = [];
          break;
        }

        pending = next;
        if (attempt < maxAttempts) {
          await sleep(baseDelayMs * attempt);
        }
      }

      unprocessedKeys.push(...pending);
    }

    return { items: allItems, unprocessedKeys };
  }

  async batchWrite(
    modelName: string,
    req: {
      puts?: Array<Record<string, unknown>>;
      deletes?: Array<Record<string, unknown>>;
    },
    opts: RetryOptions = {},
  ): Promise<BatchWriteResult> {
    const model = this.requireModel(modelName);

    const maxAttempts = opts.maxAttempts ?? 5;
    const baseDelayMs = opts.baseDelayMs ?? 25;

    const now = nowRfc3339Nano();
    const writeRequests: WriteRequest[] = [];

    for (const item of req.puts ?? []) {
      writeRequests.push({
        PutRequest: {
          Item: marshalPutItem(model, item, { now }),
        },
      });
    }

    for (const key of req.deletes ?? []) {
      writeRequests.push({
        DeleteRequest: {
          Key: marshalKey(model, key),
        },
      });
    }

    const unprocessed: WriteRequest[] = [];

    for (const requestChunk of chunk(writeRequests, 25)) {
      let pending = requestChunk;

      for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        const resp = await this.ddb.send(
          new BatchWriteItemCommand({
            RequestItems: {
              [model.tableName]: pending,
            },
          }),
        );

        const next = resp.UnprocessedItems?.[model.tableName] ?? [];
        if (next.length === 0) {
          pending = [];
          break;
        }

        pending = next;
        if (attempt < maxAttempts) {
          await sleep(baseDelayMs * attempt);
        }
      }

      unprocessed.push(...pending);
    }

    return { unprocessed };
  }

  async transactWrite(actions: TransactAction[]): Promise<void> {
    const transactItems: TransactWriteItem[] = actions.map(
      (a): TransactWriteItem => {
        const model = this.requireModel(a.model);

        switch (a.kind) {
          case 'put': {
            const put: Put = {
              TableName: model.tableName,
              Item: marshalPutItem(model, a.item),
            };
            if (a.ifNotExists) {
              put.ConditionExpression = 'attribute_not_exists(#pk)';
              put.ExpressionAttributeNames = { '#pk': model.roles.pk };
            }
            return { Put: put };
          }
          case 'delete':
            return {
              Delete: {
                TableName: model.tableName,
                Key: marshalKey(model, a.key),
              } satisfies Delete,
            };
          case 'condition':
            return {
              ConditionCheck: {
                TableName: model.tableName,
                Key: marshalKey(model, a.key),
                ConditionExpression: a.conditionExpression,
                ExpressionAttributeNames: a.expressionAttributeNames,
                ExpressionAttributeValues: a.expressionAttributeValues,
              } satisfies ConditionCheck,
            };
          default:
            throw new DynamormError(
              'ErrInvalidOperator',
              'Unknown transaction action',
            );
        }
      },
    );

    try {
      await this.ddb.send(
        new TransactWriteItemsCommand({ TransactItems: transactItems }),
      );
    } catch (err) {
      throw mapDynamoError(err);
    }
  }

  query(modelName: string): QueryBuilder {
    const model = this.requireModel(modelName);
    return new QueryBuilder(this.ddb, model);
  }

  scan(modelName: string): ScanBuilder {
    const model = this.requireModel(modelName);
    return new ScanBuilder(this.ddb, model);
  }
}

function mapDynamoError(err: unknown): unknown {
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
