import assert from "node:assert/strict";
import { sessionCatalogNotice } from "../lib/sessionCatalogPresentation";
import type { SessionCatalogStatus } from "../lib/sessionCatalogTypes";

const status = (overrides: Partial<SessionCatalogStatus> = {}): SessionCatalogStatus => ({
  state: "ready",
  revision: 1,
  indexed: 10,
  total: 10,
  repairPending: 0,
  canRebuild: false,
  ...overrides,
});

assert.deepEqual(
  sessionCatalogNotice(status({ state: "opening", indexed: 0, total: 0 })),
  "indexing",
  "opening without a known total uses the generic working label",
);
assert.deepEqual(
  sessionCatalogNotice(status({ state: "rebuilding", indexed: 7, total: 10, canRebuild: true })),
  "indexing",
  "rebuilding preserves known progress and never offers another rebuild",
);
assert.deepEqual(
  sessionCatalogNotice(status({ repairPending: 1, canRebuild: true })),
  "repair-active",
  "older backends map their aggregate repair backlog to active repair",
);
assert.equal(
  sessionCatalogNotice(status({ repairPending: 3, repairActive: 0, repairDeferred: 3, repairBlocked: 0 })),
  "repair-deferred",
  "deferred repair is static and does not reuse indexing progress",
);
assert.equal(
  sessionCatalogNotice(status({ repairPending: 2, repairActive: 0, repairDeferred: 0, repairBlocked: 2 })),
  "repair-blocked",
  "blocked repair is distinct from active work",
);
assert.equal(
  sessionCatalogNotice(status({ repairPending: 4, repairActive: 1, repairDeferred: 2, repairBlocked: 1 })),
  "repair-active",
  "active work takes precedence while a mixed backlog is running",
);
assert.equal(sessionCatalogNotice(status()), null, "healthy ready catalogs render no notice");
assert.deepEqual(
  sessionCatalogNotice(status({ state: "degraded", canRebuild: true })),
  "rebuild",
  "a degraded catalog can expose the explicit repair action",
);
assert.deepEqual(
  sessionCatalogNotice(status({ lastError: "private backend detail", canRebuild: true })),
  "rebuild",
  "backend errors select a generic failed presentation without exposing the detail",
);
assert.deepEqual(
  sessionCatalogNotice(status({ state: "degraded", canRebuild: undefined })),
  "failed",
  "older backends without canRebuild fail closed",
);

console.log("session catalog presentation tests passed");
