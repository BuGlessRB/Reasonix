export interface DecisionModels {
  typeSafe: { baseUrl: string; model: string; keyEnv: string; hasKey: boolean };
  laya: { local: boolean; python: string; model: string; httpBaseUrl: string; httpKeyEnv: string; hasHttpKey: boolean };
}

export interface DecisionModelsDraft extends DecisionModels {
  typeSafe: DecisionModels["typeSafe"] & { apiKey?: string };
  laya: DecisionModels["laya"] & { httpApiKey?: string };
}
