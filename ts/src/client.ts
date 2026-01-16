import {
  type AttributeValue,
  ConditionalCheckFailedException,
  DeleteItemCommand,
  DynamoDBClient,
  GetItemCommand,
  PutItemCommand,
  UpdateItemCommand,
} from '@aws-sdk/client-dynamodb';

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

  if (typeof err === 'object' && err !== null && 'name' in err) {
    const name = (err as { name?: unknown }).name;
    if (name === 'ConditionalCheckFailedException') {
      return new DynamormError('ErrConditionFailed', 'Condition failed', {
        cause: err,
      });
    }
  }

  return err;
}
