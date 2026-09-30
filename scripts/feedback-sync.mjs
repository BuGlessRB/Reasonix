#!/usr/bin/env node
// Moves in-app feedback into GitHub issues and reports issue state back to the
// worker. The worker is the queue; GitHub is the only writer to GitHub.
//
// Usage:
//   GH_TOKEN=... FEEDBACK_ADMIN_TOKEN=... node scripts/feedback-sync.mjs [--dry-run] [--full] [--limit N]
//
// Status sync is windowed: only issues updated within LOOKBACK_MINUTES are
// re-read (plus entries fixed as "next", which wait for a tag). --full re-reads
// every open entry and is the recovery path after an outage longer than the window.

import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

export const REPO = "esengine/DeepSeek-Reasonix";
export const DEFAULT_BASE = "https://crash.reasonix.io";
export const RECEIPT = /^FB-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$/;
export const ATTACHMENT_PREFIX = `${DEFAULT_BASE}/v1/feedback/attachments/`;
export const CATEGORY_LABEL = { bug: "bug", idea: "enhancement", question: "question", other: null };
export const SOURCE_LABEL = { name: "from-studio", color: "5319e7", description: "Filed from inside Reasonix Studio" };
export const BOT_LOGIN = "github-actions[bot]";
export const LOOKBACK_MINUTES = 120;
export const MAX_OPEN = 200;
export const IN_PROGRESS_LABEL = "in-progress";
export const CATEGORY_LABEL_COLORS = { bug: "d73a4a", enhancement: "a2eeef", question: "d876e3" };
const ENV_FIELDS = ["version", "commit", "surface", "os", "osVersion", "arch", "locale", "channel", "providerKind"];
const ZWSP = "​";

export const marker = (receipt) => `<!-- reasonix-feedback: ${receipt} -->`;

// GitHub decodes entities before it looks for mentions and references, so
// escaping cannot make text inert. Code spans and fences are the one place it
// recognises none of them: every string a user wrote goes in one.
export function fence(text) {
  const longest = Math.max(0, ...[...String(text).matchAll(/`+/g)].map((m) => m[0].length));
  const ticks = "`".repeat(Math.max(3, longest + 1));
  return `${ticks}text\n${text}\n${ticks}`;
}

// Single-line values (nickname, environment): no backticks or pipes to break out of.
export const code = (text, max) => `\`${String(text ?? "").replace(/[\s`|]+/g, " ").trim().slice(0, max)}\``;

export function renderTitle(body) {
  const first = String(body ?? "").split(/\r?\n/).find((l) => l.trim()) ?? "";
  const line = first.replace(/\s+/g, " ").trim().slice(0, 80);
  return `[Studio feedback] ${line.replace(/@/g, `@${ZWSP}`).replace(/#(?=\d)/g, `#${ZWSP}`).replace(/(?<=[A-Za-z])-(?=\d)/g, `-${ZWSP}`)}`;
}

export function attachmentUrls(item) {
  const urls = [];
  for (const a of item.attachments ?? []) {
    const url = typeof a === "string" ? a : a?.url;
    if (typeof url === "string" && url.startsWith(ATTACHMENT_PREFIX) && !/[\s()<>]/.test(url)) {
      urls.push({ name: typeof a === "string" ? "image" : a.name || "image", url });
    }
  }
  return urls;
}

export function renderBody(item) {
  const shots = attachmentUrls(item).map((a, i) => `![image ${i + 1}](${a.url})`);
  const env = item.env ?? {};
  const rows = ENV_FIELDS.filter((k) => env[k] != null && env[k] !== "").map((k) => `| ${k} | ${code(env[k], 100)} |`);
  const parts = [`Reported by ${code(item.displayName, 40)} via Reasonix Studio (receipt ${item.receipt})`, fence(String(item.body ?? "").replace(/\r\n?/g, "\n"))];
  if (shots.length) parts.push(shots.join("\n\n"));
  if (rows.length) parts.push(`<details><summary>Environment</summary>\n\n| Field | Value |\n| --- | --- |\n${rows.join("\n")}\n\n</details>`);
  parts.push(marker(item.receipt));
  return parts.join("\n\n");
}

// The marker is the last line of a body we wrote; text before it cannot forge it.
export const hasMarker = (body, receipt) => (body ?? "").trimEnd().endsWith(marker(receipt));

export function labelsFor(item) {
  const cat = CATEGORY_LABEL[item.category] ?? null;
  return cat ? [SOURCE_LABEL.name, cat] : [SOURCE_LABEL.name];
}

const CLOSING = /\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+#(\d+)\b/gi;
export const closesIssue = (body, n) => [...(body ?? "").matchAll(CLOSING)].some((m) => Number(m[1]) === n);

const isRepoPull = (src) => src?.pull_request && (src.repository?.full_name ?? "").toLowerCase() === REPO.toLowerCase();

// Pull requests of this repository that name the issue with a closing keyword.
// A bare mention, or a reference from another repository, proves nothing.
export function closingPulls(number, timeline) {
  const out = new Map();
  for (const e of timeline) {
    const src = e.event === "cross-referenced" ? e.source?.issue : null;
    if (isRepoPull(src) && closesIssue(src.body, number)) out.set(src.number, src);
  }
  return [...out.values()];
}

// The close event's own commit wins; otherwise the earliest merged closing PR.
export function closingCommit(number, timeline, pulls) {
  const event = [...timeline].reverse().find((e) => e.event === "closed");
  if (event?.commit_id) return event.commit_id;
  const merged = closingPulls(number, timeline).map((s) => pulls[s.number]).filter((pr) => pr?.merged_at && pr.merge_commit_sha);
  merged.sort((a, b) => Date.parse(a.merged_at) - Date.parse(b.merged_at));
  return merged[0]?.merge_commit_sha ?? null;
}

export function hasOpenLinkedPull(number, timeline) {
  return closingPulls(number, timeline).some((s) => s.state === "open" && !s.pull_request.merged_at);
}

export function newestStudioRelease(releases) {
  const times = releases.filter((r) => !r.draft && /^studio-v/.test(r.tag_name) && r.published_at).map((r) => Date.parse(r.published_at));
  return times.length ? Math.max(...times) : null;
}

export function firstTag(tags) {
  const list = tags.filter((t) => /^studio-v\d+\.\d+\.\d+$/.test(t));
  const key = (t) => t.slice(8).split(".").map(Number);
  list.sort((a, b) => {
    const x = key(a), y = key(b);
    return x[0] - y[0] || x[1] - y[1] || x[2] - y[2];
  });
  return list[0] ? list[0].slice(7) : null;
}

// Returns the update to POST, or null when the worker already holds this state.
// Duplicate is a maintainer act (label or close reason), never a text match.
// A "next" fix upgrades in place once its tag exists.
export function decideStatus({ current, resolvedVersion, issue, version, inProgress }) {
  const labels = (issue.labels ?? []).map((l) => (typeof l === "string" ? l : l.name));
  if (current === "fixed") {
    return resolvedVersion === "next" && version && issue.state === "closed" ? { status: "fixed", resolvedVersion: version } : null;
  }
  let next;
  if (issue.state === "closed") {
    if (issue.state_reason === "duplicate" || labels.includes("duplicate")) next = { status: "duplicate" };
    else if (issue.state_reason === "not_planned") next = { status: "wontfix" };
    else next = { status: "fixed", resolvedVersion: version ?? "next" };
  } else if (inProgress || labels.includes(IN_PROGRESS_LABEL)) next = { status: "in_progress" };
  else return null;
  if (next.status === current) return null;
  if (next.status === "in_progress" && current !== "recorded") return null;
  return next;
}

export const isOurs = (issue) => issue?.user?.login === BOT_LOGIN && (issue.labels ?? []).some((l) => (typeof l === "string" ? l : l.name) === SOURCE_LABEL.name);

export class FatalError extends Error {}
const PROGRAMMER = new Set(["TypeError", "ReferenceError", "SyntaxError", "RangeError"]);
const isFatal = (err) => err instanceof FatalError || PROGRAMMER.has(err?.name);

async function createOne(item, deps, dry, state) {
  const { receipt } = item;
  state.recent ??= await deps.gh.recentIssues();
  let issue = state.recent.find((i) => isOurs(i) && hasMarker(i.body, receipt));
  if (!issue) {
    if (dry) return deps.log(`[dry-run] would create issue for ${receipt}: ${renderTitle(item.body)}`);
    await deps.gh.ensureLabels(labelsFor(item));
    issue = await deps.gh.createIssue({ title: renderTitle(item.body), body: renderBody(item), labels: labelsFor(item) });
    state.recent.unshift({ ...issue, user: { login: BOT_LOGIN }, labels: labelsFor(item), body: renderBody(item) });
    deps.log(`created #${issue.number} for ${receipt}`);
  } else deps.log(`found #${issue.number} for ${receipt}`);
  if (dry) return deps.log(`[dry-run] would record ${receipt} -> #${issue.number}`);
  await deps.worker.recorded(receipt, { issueNumber: issue.number, issueUrl: issue.html_url });
}

async function syncOne(row, issue, deps, dry) {
  const number = Number(row.issueNumber);
  if (issue.pull_request) return;
  const timeline = await deps.gh.timeline(number);
  let version = null;
  if (issue.state === "closed" && issue.state_reason !== "not_planned" && issue.state_reason !== "duplicate") {
    const pulls = {};
    for (const src of closingPulls(number, timeline)) pulls[src.number] = await deps.gh.getPull(src.number);
    const sha = closingCommit(number, timeline, pulls);
    version = sha ? firstTag(await deps.tagsContaining(sha)) : null;
  }
  const update = decideStatus({ current: row.status, resolvedVersion: row.resolvedVersion, issue, version, inProgress: hasOpenLinkedPull(number, timeline) });
  if (!update) return;
  if (dry) return deps.log(`[dry-run] would set ${row.receipt} (#${number}) ${JSON.stringify(update)}`);
  await deps.worker.status(row.receipt, update);
  deps.log(`${row.receipt} (#${number}) -> ${update.status}${update.resolvedVersion ? ` ${update.resolvedVersion}` : ""}`);
}

export async function run(deps, { dryRun = false, limit = 20, full = false, now = Date.now() } = {}) {
  const failures = [];
  const each = async (label, rows, fn) => {
    for (const row of rows) {
      try {
        await fn(row);
      } catch (err) {
        if (isFatal(err)) throw err;
        failures.push(`${label} ${row.receipt}: ${err.message}`);
        deps.log(`::warning::${label} ${row.receipt}: ${err.message}`);
      }
    }
  };
  const guarded = async (label, fn) => {
    try {
      return await fn();
    } catch (err) {
      if (isFatal(err)) throw err;
      failures.push(`${label}: ${err.message}`);
      deps.log(`::warning::${label}: ${err.message}`);
      return null;
    }
  };
  const state = {};
  const pending = ((await guarded("pending", () => deps.worker.pending(limit))) ?? []).slice(0, limit).filter((i) => RECEIPT.test(i.receipt));
  await each("create", pending, (item) => createOne(item, deps, dryRun, state));

  const open = ((await guarded("open", () => deps.worker.open())) ?? []).slice(0, MAX_OPEN).filter((r) => RECEIPT.test(r.receipt) && Number.isInteger(Number(r.issueNumber)) && Number(r.issueNumber) > 0);
  const waiting = (r) => r.status === "fixed" && r.resolvedVersion === "next";
  // A "next" fix can only have gained a tag if a Studio release was published lately; the daily sweep covers the rest.
  const releaseAt = open.some(waiting) && !full ? await guarded("releases", () => deps.newestReleaseAt()) : null;
  const releaseFresh = full || (releaseAt != null && releaseAt >= now - LOOKBACK_MINUTES * 60000);
  let changed = new Map();
  const skip = (r) => waiting(r) && !releaseFresh;
  if (!full && open.some((r) => !waiting(r))) {
    const since = new Date(now - LOOKBACK_MINUTES * 60000).toISOString();
    const listed = await guarded("list", () => deps.gh.changedSince(since));
    if (!listed) return { failures };
    changed = new Map(listed.map((i) => [i.number, i]));
  }
  await each("sync", open, async (row) => {
    const number = Number(row.issueNumber);
    if (skip(row)) return;
    const issue = full || waiting(row) ? await deps.gh.getIssue(number) : changed.get(number);
    if (issue) await syncOne(row, issue, deps, dryRun);
  });
  return { failures };
}

function gh(args, input) {
  const out = execFileSync("gh", args, { input, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
  return out ? JSON.parse(out) : null;
}
const paginate = (path) => {
  const out = execFileSync("gh", ["api", "--paginate", "--slurp", path], { encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
  return JSON.parse(out).flat();
};

export function liveDeps(env) {
  const base = (env.FEEDBACK_BASE_URL || DEFAULT_BASE).replace(/\/$/, "");
  const call = async (method, path, body) => {
    let res;
    try {
      res = await fetch(`${base}${path}`, {
        method,
        headers: { authorization: `Bearer ${env.FEEDBACK_ADMIN_TOKEN}`, ...(body ? { "content-type": "application/json" } : {}) },
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch (err) {
      throw new Error(`${method} ${path} unreachable: ${err.cause?.code ?? err.message}`);
    }
    if (res.status === 401 || res.status === 403) throw new FatalError(`${method} ${path} -> ${res.status} (check FEEDBACK_ADMIN_TOKEN)`);
    if (!res.ok) throw new Error(`${method} ${path} -> ${res.status}`);
    return res.status === 204 ? null : res.json().catch(() => null);
  };
  return {
    log: (m) => console.log(m),
    worker: {
      pending: async (limit) => (await call("GET", `/v1/admin/feedback/pending?limit=${limit}`)).items ?? [],
      open: async () => (await call("GET", "/v1/admin/feedback/open")).items ?? [],
      recorded: (r, b) => call("POST", `/v1/admin/feedback/${r}/recorded`, b),
      status: (r, b) => call("POST", `/v1/admin/feedback/${r}/status`, b),
    },
    gh: {
      recentIssues: async () => gh(["api", `repos/${REPO}/issues?labels=${SOURCE_LABEL.name}&state=all&sort=created&direction=desc&per_page=100`]),
      changedSince: async (since) => paginate(`repos/${REPO}/issues?labels=${SOURCE_LABEL.name}&state=all&since=${since}&per_page=100`),
      async ensureLabels(names) {
        const have = new Set(paginate(`repos/${REPO}/labels?per_page=100`).map((l) => l.name));
        for (const name of names) {
          if (have.has(name)) continue;
          const spec = name === SOURCE_LABEL.name ? SOURCE_LABEL : { name, color: CATEGORY_LABEL_COLORS[name] ?? "ededed" };
          gh(["api", "-X", "POST", `repos/${REPO}/labels`, "--input", "-"], JSON.stringify(spec));
        }
      },
      createIssue: async (payload) => gh(["api", "-X", "POST", `repos/${REPO}/issues`, "--input", "-"], JSON.stringify(payload)),
      getIssue: async (n) => gh(["api", `repos/${REPO}/issues/${n}`]),
      timeline: async (n) => paginate(`repos/${REPO}/issues/${n}/timeline?per_page=100`),
      getPull: async (n) => gh(["api", `repos/${REPO}/pulls/${n}`]),
    },
    async newestReleaseAt() {
      const releases = gh(["api", `repos/${REPO}/releases?per_page=10`]) ?? [];
      return newestStudioRelease(releases);
    },
    async tagsContaining(sha) {
      if (!/^[0-9a-f]{40}$/.test(sha)) return [];
      try {
        return execFileSync("git", ["tag", "--contains", sha, "--list", "studio-v*"], { encoding: "utf8" }).split("\n").filter(Boolean);
      } catch {
        return [];
      }
    },
  };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const dryRun = args.includes("--dry-run");
  const li = args.indexOf("--limit");
  const limit = li >= 0 ? Number(args[li + 1]) || 20 : 20;
  const full = args.includes("--full");
  if (!process.env.FEEDBACK_ADMIN_TOKEN) {
    console.log("::notice::FEEDBACK_ADMIN_TOKEN is not set; feedback sync skipped");
    process.exit(0);
  }
  const { failures } = await run(liveDeps(process.env), { dryRun, limit, full });
  if (failures.length) console.log(`${failures.length} item(s) skipped; they are retried on the next run`);
}
