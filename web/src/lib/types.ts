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
  /** Data one unit moves over the edge, both ways; read between nodes of one group. */
  kb?: number;
  note?: string;
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
  warnings: string[] | null;
}

export interface TerraformResponse {
  spec: Spec;
  warnings: string[];
}

/** One provider's regions, as the price books cover them. */
export interface RegionGroup {
  provider: string;
  label: string;
  regions: string[];
}
