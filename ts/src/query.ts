import {
  DynamoDBClient,
  QueryCommand,
  ScanCommand,
  type AttributeValue,
} from '@aws-sdk/client-dynamodb';

import {
  decodeCursor,
  encodeCursor,
  type Cursor,
  type CursorSort,
} from './cursor.js';
import { DynamormError } from './errors.js';
import { marshalScalar, unmarshalItem } from './marshal.js';
import type { AttributeSchema, IndexSchema, Model } from './model.js';

export interface Page<T = Record<string, unknown>> {
  items: T[];
  cursor?: string;
}

export class QueryBuilder {
  private indexName?: string;
  private pkValue?: unknown;
  private skCondition?: {
    op: '=' | '<' | '<=' | '>' | '>=' | 'between' | 'begins_with';
    values: unknown[];
  };
  private limitCount?: number;
  private projectionFields?: string[];
  private consistentReadEnabled = false;
  private cursorToken?: string;
  private sortDir: CursorSort = 'ASC';

  constructor(
    private readonly ddb: DynamoDBClient,
    private readonly model: Model,
  ) {}

  usingIndex(name: string): this {
    this.indexName = name;
    return this;
  }

  sort(direction: CursorSort): this {
    this.sortDir = direction;
    return this;
  }

  consistentRead(enabled = true): this {
    this.consistentReadEnabled = enabled;
    return this;
  }

  limit(n: number): this {
    this.limitCount = n;
    return this;
  }

  projection(fields: string[]): this {
    this.projectionFields = fields.slice();
    return this;
  }

  cursor(encoded: string): this {
    this.cursorToken = encoded;
    return this;
  }

  partitionKey(value: unknown): this {
    this.pkValue = value;
    return this;
  }

  sortKey(
    op: '=' | '<' | '<=' | '>' | '>=' | 'between' | 'begins_with',
    ...values: unknown[]
  ): this {
    this.skCondition = { op, values };
    return this;
  }

  async page(): Promise<Page> {
    const { pkName, pkSchema, skName, skSchema, index } =
      this.resolveKeySchema();
    if (this.pkValue === undefined)
      throw new DynamormError(
        'ErrInvalidOperator',
        'partitionKey() is required',
      );

    if (this.indexName && this.consistentReadEnabled) {
      throw new DynamormError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }

    const names: Record<string, string> = { '#pk': pkName };
    const values: Record<string, AttributeValue> = {
      ':pk': marshalScalar(pkSchema, this.pkValue),
    };

    let keyExpr = '#pk = :pk';
    if (this.skCondition) {
      if (!skName || !skSchema)
        throw new DynamormError(
          'ErrInvalidOperator',
          'sortKey() requires a sort key',
        );
      names['#sk'] = skName;

      const { op, values: skValues } = this.skCondition;
      switch (op) {
        case 'begins_with': {
          if (skValues.length !== 1)
            throw new DynamormError(
              'ErrInvalidOperator',
              'begins_with requires one value',
            );
          values[':sk'] = marshalScalar(skSchema, skValues[0]);
          keyExpr += ' AND begins_with(#sk, :sk)';
          break;
        }
        case 'between': {
          if (skValues.length !== 2)
            throw new DynamormError(
              'ErrInvalidOperator',
              'between requires two values',
            );
          values[':sk0'] = marshalScalar(skSchema, skValues[0]);
          values[':sk1'] = marshalScalar(skSchema, skValues[1]);
          keyExpr += ' AND #sk BETWEEN :sk0 AND :sk1';
          break;
        }
        default: {
          if (skValues.length !== 1)
            throw new DynamormError(
              'ErrInvalidOperator',
              'sort operator requires one value',
            );
          values[':sk'] = marshalScalar(skSchema, skValues[0]);
          keyExpr += ` AND #sk ${op} :sk`;
          break;
        }
      }
    }

    let projectionExpr: string | undefined;
    if (this.projectionFields?.length) {
      const projParts: string[] = [];
      for (let i = 0; i < this.projectionFields.length; i++) {
        const field = this.projectionFields[i]!;
        const placeholder = `#p${i}`;
        names[placeholder] = field;
        projParts.push(placeholder);
      }
      projectionExpr = projParts.join(', ');
    }

    let exclusiveStartKey: Record<string, AttributeValue> | undefined;
    if (this.cursorToken) {
      const c = decodeCursor(this.cursorToken);
      if (c.index && (this.indexName ?? undefined) !== c.index) {
        throw new DynamormError(
          'ErrInvalidOperator',
          'Cursor index does not match query',
        );
      }
      if (c.sort && c.sort !== this.sortDir) {
        throw new DynamormError(
          'ErrInvalidOperator',
          'Cursor sort does not match query',
        );
      }
      exclusiveStartKey = c.lastKey;
    }

    const resp = await this.ddb.send(
      new QueryCommand({
        TableName: this.model.tableName,
        IndexName: index?.name,
        KeyConditionExpression: keyExpr,
        ExpressionAttributeNames: names,
        ExpressionAttributeValues: values,
        Limit: this.limitCount,
        ProjectionExpression: projectionExpr,
        ConsistentRead: this.consistentReadEnabled || undefined,
        ExclusiveStartKey: exclusiveStartKey,
        ScanIndexForward: this.sortDir === 'ASC',
      }),
    );

    const items = (resp.Items ?? []).map((it) => unmarshalItem(this.model, it));
    let cursor: string | undefined;
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey, sort: this.sortDir };
      if (index) c.index = index.name;
      cursor = encodeCursor(c);
    }

    const page: Page = { items };
    if (cursor) page.cursor = cursor;
    return page;
  }

  private resolveKeySchema(): {
    pkName: string;
    pkSchema: Readonly<AttributeSchema>;
    skName?: string;
    skSchema?: Readonly<AttributeSchema>;
    index?: IndexSchema;
  } {
    if (this.indexName) {
      const index = this.model.indexes.get(this.indexName);
      if (!index)
        throw new DynamormError(
          'ErrInvalidOperator',
          `Unknown index: ${this.indexName}`,
        );

      const pkName = index.partition.attribute;
      const pkSchema = this.model.attributes.get(pkName);
      if (!pkSchema)
        throw new DynamormError(
          'ErrInvalidModel',
          `Index pk attribute missing: ${pkName}`,
        );

      const skName = index.sort?.attribute;
      const out: {
        pkName: string;
        pkSchema: Readonly<AttributeSchema>;
        skName?: string;
        skSchema?: Readonly<AttributeSchema>;
        index: IndexSchema;
      } = { pkName, pkSchema, index };

      if (skName) {
        const skSchema = this.model.attributes.get(skName);
        if (!skSchema)
          throw new DynamormError(
            'ErrInvalidModel',
            `Index sk attribute missing: ${skName}`,
          );
        out.skName = skName;
        out.skSchema = skSchema;
      }

      return out;
    }

    const pkName = this.model.roles.pk;
    const pkSchema = this.model.attributes.get(pkName);
    if (!pkSchema)
      throw new DynamormError(
        'ErrInvalidModel',
        `Model pk attribute missing: ${pkName}`,
      );

    const out: {
      pkName: string;
      pkSchema: Readonly<AttributeSchema>;
      skName?: string;
      skSchema?: Readonly<AttributeSchema>;
    } = { pkName, pkSchema };

    const skName = this.model.roles.sk;
    if (skName) {
      const skSchema = this.model.attributes.get(skName);
      if (!skSchema)
        throw new DynamormError(
          'ErrInvalidModel',
          `Model sk attribute missing: ${skName}`,
        );
      out.skName = skName;
      out.skSchema = skSchema;
    }

    return out;
  }
}

export class ScanBuilder {
  private indexName?: string;
  private limitCount?: number;
  private projectionFields?: string[];
  private consistentReadEnabled = false;
  private cursorToken?: string;

  constructor(
    private readonly ddb: DynamoDBClient,
    private readonly model: Model,
  ) {}

  usingIndex(name: string): this {
    this.indexName = name;
    return this;
  }

  consistentRead(enabled = true): this {
    this.consistentReadEnabled = enabled;
    return this;
  }

  limit(n: number): this {
    this.limitCount = n;
    return this;
  }

  projection(fields: string[]): this {
    this.projectionFields = fields.slice();
    return this;
  }

  cursor(encoded: string): this {
    this.cursorToken = encoded;
    return this;
  }

  async page(): Promise<Page> {
    if (this.indexName && this.consistentReadEnabled) {
      throw new DynamormError(
        'ErrInvalidOperator',
        'Consistent reads are not supported on GSIs',
      );
    }

    const names: Record<string, string> = {};
    let projectionExpr: string | undefined;
    if (this.projectionFields?.length) {
      const projParts: string[] = [];
      for (let i = 0; i < this.projectionFields.length; i++) {
        const field = this.projectionFields[i]!;
        const placeholder = `#p${i}`;
        names[placeholder] = field;
        projParts.push(placeholder);
      }
      projectionExpr = projParts.join(', ');
    }

    let exclusiveStartKey: Record<string, AttributeValue> | undefined;
    if (this.cursorToken) {
      const c = decodeCursor(this.cursorToken);
      if (c.index && (this.indexName ?? undefined) !== c.index) {
        throw new DynamormError(
          'ErrInvalidOperator',
          'Cursor index does not match scan',
        );
      }
      exclusiveStartKey = c.lastKey;
    }

    const resp = await this.ddb.send(
      new ScanCommand({
        TableName: this.model.tableName,
        IndexName: this.indexName,
        Limit: this.limitCount,
        ProjectionExpression: projectionExpr,
        ExpressionAttributeNames: Object.keys(names).length ? names : undefined,
        ConsistentRead: this.consistentReadEnabled || undefined,
        ExclusiveStartKey: exclusiveStartKey,
      }),
    );

    const items = (resp.Items ?? []).map((it) => unmarshalItem(this.model, it));
    let cursor: string | undefined;
    if (resp.LastEvaluatedKey) {
      const c: Cursor = { lastKey: resp.LastEvaluatedKey };
      if (this.indexName) c.index = this.indexName;
      cursor = encodeCursor(c);
    }

    const page: Page = { items };
    if (cursor) page.cursor = cursor;
    return page;
  }
}
