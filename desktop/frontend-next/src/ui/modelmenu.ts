// modelmenu.ts — the model picker's rows, grouped the way the account is.
import type { ModelEntry } from "../port/port";
import type { MenuItem } from "./Menu";
import { groupVendors } from "./Models";
import { t } from "../i18n";
import { KIND_LABEL } from "./vendors";

// One flat list said "deepseek" four times: the provider name is the config's
// word for an entry, not the user's for an endpoint, and two doors onto one
// account share it. Rows carry the account they belong to and the protocol that
// reaches them — but only where either is ambiguous, so a single source stays a
// plain list of models.
export function modelMenu(models: ModelEntry[]): MenuItem[] {
  const accounts = groupVendors(models);
  const out: MenuItem[] = [];
  for (const [i, a] of accounts.entries()) {
    const manyDoors = a.kinds.length > 1;
    const label = accounts.length > 1 || manyDoors;
    if (label) {
      out.push({ value: `__account:${a.key}`, label: a.label, right: a.host, header: true, divide: i > 0 });
    }
    for (const kind of a.kinds) {
      for (const m of a.byKind[kind]) {
        out.push({
          value: m.ref,
          label: m.model,
          right: manyDoors ? t(KIND_LABEL[kind] ?? kind) : undefined,
        });
      }
    }
  }
  return out;
}
