import type { SessionCatalogStatus } from "./sessionCatalogTypes";

export type SessionCatalogNotice = "indexing" | "repair-active" | "repair-deferred" | "repair-blocked" | "failed" | "rebuild";

export function sessionCatalogNotice(status: SessionCatalogStatus): SessionCatalogNotice | null {
  const indexing = status.state === "opening"
    || status.state === "rebuilding"
    || (status.unindexedTargetCount ?? 0) > 0;
  if (indexing) return "indexing";
  const hasDetailedRepairState = status.repairActive !== undefined
    || status.repairDeferred !== undefined
    || status.repairBlocked !== undefined;
  if ((status.repairActive ?? (!hasDetailedRepairState ? status.repairPending : 0)) > 0) return "repair-active";
  if ((status.repairDeferred ?? 0) > 0) return "repair-deferred";
  if ((status.repairBlocked ?? 0) > 0) return "repair-blocked";
  if (status.state === "degraded" || status.lastError) return status.canRebuild === true ? "rebuild" : "failed";
  return null;
}
