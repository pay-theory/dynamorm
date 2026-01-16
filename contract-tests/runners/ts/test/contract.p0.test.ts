import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { loadModelsDir, loadScenariosDir } from "../src/load.js";
import { NotImplementedDriver } from "../src/driver.js";

function contractRoot(): string {
  const __dirname = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(__dirname, "..", "..", ".."); // runners/ts/test -> contract-tests
}

test("P0 contract suite harness loads (driver stub)", async () => {
  const root = contractRoot();
  const models = await loadModelsDir(path.join(root, "dms", "v0.1", "models"));
  const scenarios = await loadScenariosDir(path.join(root, "scenarios", "p0"));
  assert.ok(models.size > 0);
  assert.ok(scenarios.length > 0);

  // This is a placeholder until dynamorm-ts implements a real Driver.
  const driver = new NotImplementedDriver();
  assert.ok(driver);
});

