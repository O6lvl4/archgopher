// Mirrors the JSON of the Go packages scout and api. Go nil slices arrive as null.

export interface Load {
  monthly: number;
  peakPerSecond: number;
}

export interface Position {
  x: number;
  y: number;
}

export type Values = Record<string, unknown>;

export interface SpecNode {
  id: string;
  type: string;
  address?: string;
  attributes?: Values;
  assumptions?: Values;
  load?: Load;
  note?: string;
  position?: Position;
  stale?: boolean;
}

export interface SpecEdge {
  from: string;
  to: string;
  kind?: string;
  perUnit?: number;
  note?: string;
}

export interface Spec {
  name: string;
  region: string;
  nodes: SpecNode[];
  edges: SpecEdge[];
}

export type FieldType = "number" | "string" | "boolean" | "list" | "choice";

export interface Field {
  key: string;
  label: string;
  type: FieldType;
  unit?: string;
  hint?: string;
  default?: unknown;
  options?: string[];
  multi?: boolean;
  required: boolean;
  path?: string;
}

export interface CatalogEntry {
  type: string;
  label: string;
  category: string;
  description: string;
  kinds: string[];
  sla?: string;
  external?: boolean;
  attributes: Field[] | null;
  assumptions: Field[] | null;
}

export interface Cost {
  name: string;
  quantity: number;
  unit: string;
  priceId: string;
  unitPrice: number | null;
  monthlyUsd: number | null;
}

export interface Limit {
  name: string;
  unit: string;
  demand: number;
  quotaId?: string;
  capacity: number | null;
  headroom: number | null;
  from: string;
}

export interface NodeResult {
  id: string;
  type: string;
  label: string;
  address?: string;
  note?: string;
  stale?: boolean;
  demand: Record<string, Load> | null;
  costs: Cost[] | null;
  limits: Limit[] | null;
  latency?: { p50Ms: number; p99Ms: number };
  sla?: { id: string; value: number | null };
  monthlyUsd: number;
  skipped?: string;
  error?: string;
}

export interface PathResult {
  nodes: string[];
  p50Ms: number;
  p99Ms: number;
  availability: number;
  missingLatency: string[];
  missingSla: string[];
}

export interface RefUse {
  book: string;
  id: string;
  region: string;
  verified: boolean;
  known: boolean;
  source: string;
}

export interface Result {
  name: string;
  region: string;
  nodes: NodeResult[] | null;
  paths: PathResult[] | null;
  monthlyUsd: number;
  unpricedCosts: number;
  unverified: RefUse[] | null;
  warnings: string[] | null;
}

export interface TerraformResponse {
  spec: Spec;
  warnings: string[];
}
