import type { Scenario } from "./types.js";

export type ErrorCode =
  | "ErrItemNotFound"
  | "ErrConditionFailed"
  | "ErrInvalidModel"
  | "ErrMissingPrimaryKey"
  | "ErrInvalidOperator";

export interface Driver {
  create(model: string, item: Record<string, unknown>, opts: { ifNotExists?: boolean }): Promise<void>;
  get(model: string, key: Record<string, unknown>): Promise<Record<string, unknown>>;
  update(model: string, item: Record<string, unknown>, fields: string[]): Promise<void>;
  delete(model: string, key: Record<string, unknown>): Promise<void>;
}

export class NotImplementedDriver implements Driver {
  async create(_model: string, _item: Record<string, unknown>, _opts: { ifNotExists?: boolean }): Promise<void> {
    throw new Error("NotImplementedDriver.create");
  }
  async get(_model: string, _key: Record<string, unknown>): Promise<Record<string, unknown>> {
    throw new Error("NotImplementedDriver.get");
  }
  async update(_model: string, _item: Record<string, unknown>, _fields: string[]): Promise<void> {
    throw new Error("NotImplementedDriver.update");
  }
  async delete(_model: string, _key: Record<string, unknown>): Promise<void> {
    throw new Error("NotImplementedDriver.delete");
  }
}

export async function runScenario(_scenario: Scenario, _driver: Driver): Promise<void> {
  // Intentionally stubbed: the TS implementation should wire scenario steps to dynamorm-ts operations.
  throw new Error("runScenario not implemented");
}

