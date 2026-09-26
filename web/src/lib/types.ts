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
  /** The id of the boundary the node sits in (a VPC). */
  group?: string;
  /** How many identical resources the node stands for (count or for_each); -1 when unknown before apply. */
  instances?: number;
  /** The load said another way; the engine turns it into a load. A node has one or the other. */
  traffic?: Traffic;
}

/** How load arrives: exactly one shape, and when it comes. */
export interface Traffic {
  rate?: { count: number; per: string };
  users?: { count: number; actions: number; per: string };
  concurrent?: { users: number; everySeconds: number };
  schedule?: string;
  batch?: { items: number; every: string; withinSeconds: number };
  hours?: string;
  days?: string;
  peakFactor?: number;
  peakPerSecond?: number;
}

/** A boundary nodes sit in: a VPC, a virtual network. */
export interface SpecGroup {
  id: string;
  kind: string;
  label?: string;
  /** The scouter that reads traffic between the group's nodes (aws_vpc). */
  type?: string;
  assumptions?: Values;
  /** Where the frame is drawn; the engine ignores it. */
  position?: Position;
  size?: { width: number; height: number };
}

export interface SpecEdge {
  from: string;
  to: string;
  kind?: string;
  perUnit?: number;
  /** Size of one operation, both ways: the target counts its billing units by it, and a group its zone crossings. */
  kb?: number;
  /** Several kinds of work per upstream unit; kind, perUnit and kb are the one-operation short form. */
  ops?: EdgeOp[];
  note?: string;
}

/** One kind of work an edge does per upstream unit. */
export interface EdgeOp {
  kind?: string;
  perUnit?: number;
  kb?: number;
}

export interface Spec {
  name: string;
  region: string;
  nodes: SpecNode[];
  edges: SpecEdge[];
  groups?: SpecGroup[];
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
  /** The cloud: "aws", "azure"; absent for provider-neutral nodes such as the entry. */
  provider?: string;
  description: string;
  kinds: string[];
  sla?: string;
  external?: boolean;
  /** The picture: "<provider>/<name>", or "general/<name>" for provider-neutral nodes. */
  icon?: string;
  /** A boundary drawn around nodes (a VPC), not placed as a node. */
  boundary?: boolean;
  attributes: Field[] | null;
  assumptions: Field[] | null;
}

export interface Cost {
  name: string;
  quantity: number;
  unit: string;
  priceId: string;
  /** With tiers or included units, what the line pays per unit on average. */
  unitPrice: number | null;
  monthlyUsd: number | null;
  /** Set when the price is billed on the whole account's usage: the key of its pool. */
  pool?: string;
}

/** The part of a quantity one price applies to: the units a plan includes, or one volume tier. */
export interface Band {
  from: number;
  to?: number;
  quantity: number;
  unitPrice: number | null;
  included?: boolean;
}

/** A price billed on the whole account's usage, and the lines that share it. */
export interface Pool {
  key: string;
  priceId: string;
  region: string;
  unit: string;
  quantity: number;
  bands: Band[] | null;
  monthlyUsd: number | null;
  members: { node: string; line: string; quantity: number }[];
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
  /** Set when the node stands for more than one resource: costs are for all of them, limits for one. */
  instances?: number;
  note?: string;
  stale?: boolean;
  demand: Record<string, Load> | null;
  /** The load the node brings in, and how its traffic was turned into it. */
  load?: Load;
  loadBasis?: string;
  costs: Cost[] | null;
  limits: Limit[] | null;
  latency?: { p50Ms: number; p99Ms: number };
  sla?: { id: string; value: number | null };
  monthlyUsd: number;
  /** Set on a pattern's rolled-up result: the member ids it sums. */
  members?: string[];
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
  /** Readings of groups with traffic between their nodes. */
  groups?: NodeResult[] | null;
  monthlyUsd: number;
  unpricedCosts: number;
  unverified: RefUse[] | null;
  /** Prices billed on the whole account's usage. */
  pools?: Pool[] | null;
  warnings: string[] | null;
}

/** Managed resources of an import by what became of them, counted per type. */
export interface Coverage {
  read: Record<string, number>;
  free: Record<string, number>;
  unpriced: Record<string, number>;
}

export interface TerraformResponse {
  spec: Spec;
  warnings: string[];
  coverage: Coverage;
}

/** One provider's regions, as the price books cover them. */
export interface RegionGroup {
  provider: string;
  label: string;
  regions: string[];
}
