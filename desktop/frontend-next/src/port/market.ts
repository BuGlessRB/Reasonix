import type { PluginPlan } from "./plugin";

// The community registry as the market tab reads it. Every text field is the
// publisher's; what an install puts on disk is decided by the approved
// version's pinned digest and the plan the person confirms, never by these.
// A theme is its own category but installs as a plugin package carrying only themes.
export type MarketKind = "skill" | "plugin" | "mcp" | "theme";

// What this machine holds of a listed package, read off the market's own
// install record — and only while the things it installed are still there.
export interface MarketInstalled {
  version: string;
  contentHash: string;
}

export interface MarketPackage {
  kind: MarketKind;
  handle: string;
  name: string;
  slug: string;
  summary: string;
  description: string;
  homepage: string;
  repoUrl: string;
  tags: string[];
  latestVersion: string;
  installCount: number;
  starCount: number;
  verified: boolean;
  status: string;
  updatedAt: string;
  installed?: MarketInstalled;
  // The registry's word that the approved version is pinned; absent when it
  // did not say. The install still checks the digest itself.
  pinned?: boolean;
}

// The version a reviewer let through. contentHash is the pin: an install is
// refused unless the source still resolves to exactly this.
export interface MarketVersion {
  version: string;
  source: string;
  contentHash: string;
  riskLevel: string;
  createdAt: string;
}

export interface MarketDetail {
  package: MarketPackage;
  approved?: MarketVersion;
  // False means the market will refuse to install it (market.unpinned).
  pinned: boolean;
  installed?: MarketInstalled;
}

export interface MarketList {
  packages: MarketPackage[];
  limit: number;
  offset: number;
}

export interface MarketQuery {
  kind?: MarketKind | "";
  q?: string;
  sort?: "trending" | "new" | "installs";
  offset?: number;
  // Filtered by the registry, so paging stays whole.
  pinned?: boolean;
}

// version is the approved version the person was shown; the kernel refuses an
// install once a different one is approved (market.version_changed).
export interface MarketRequest {
  slug: string;
  version?: string;
  planId?: string;
  replace?: boolean;
}

// unreviewed marks the publisher's own install of a version no reviewer
// pinned: contentDigest is then this preview's, and apply must echo it.
export type MarketPlan = PluginPlan & { slug: string; version: string; contentDigest?: string; unreviewed?: boolean };

// The account's own package, any review state. digest is the previewed
// contentDigest; the kernel refuses an apply without it (market.unpreviewed).
export interface MarketOwnRequest extends MarketRequest {
  digest?: string;
}

// One package offered for review under the signed-in account's handle.
export interface MarketSubmission {
  kind: MarketKind;
  name: string;
  source: string;
  summary?: string;
  description?: string;
  repoUrl?: string;
  version?: string;
  tags?: string[];
  // private keeps it to the publisher and out of review until submitted.
  visibility?: "public" | "private";
}

// The registry's receipt: a new submission lands as pending until approved.
export interface MarketPublished {
  package: MarketPackage;
  created: boolean;
  version: string;
}

export type MarketReviewStatus = "pending" | "active" | "rejected" | "hidden" | "private";
