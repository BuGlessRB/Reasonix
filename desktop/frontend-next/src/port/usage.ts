// What this machine has spent — tokens on one side, money on the other.
//
// Cost is a list rather than a number because two billing currencies never add
// up: a single total would have to invent an exchange rate, and the rate a turn
// was billed at is not the rate today. An empty list means the records carry no
// cost for that span, which is not the same as having cost nothing.
export interface Money {
  amount: string;
  currency: string;
}

export interface UsageDay {
  day: string;
  total: number;
  byModel: Record<string, number>;
  byProvider: Record<string, number>;
  requests: number;
  turns: number;
  cacheHit: number;
  cacheMiss: number;
  cost?: Money[];
}

export interface UsageModel {
  model: string;
  provider: string;
  tokens: number;
  percent: number;
}

export interface UsageProvider {
  provider: string;
  tokens: number;
  percent: number;
}

export interface UsageReport {
  from: string;
  to: string;
  tokens: number;
  requests: number;
  turns: number;
  cache_hit: number;
  cache_miss: number;
  cost?: Money[];
  active_days: number;
  top_model: string;
  top_provider: string;
  daily: UsageDay[];
  models: UsageModel[];
  providers: UsageProvider[];
}
