import notes from "../../../examples/serverless-api/notes.scouter.yaml?raw";
import orders from "../../../examples/patterns/orders.scouter.yaml?raw";
import reports from "../../../examples/private-network/reports.scouter.yaml?raw";

/** The bundled examples, the same files the CLI tests read. */
export const examples: Record<string, string> = {
  "Notes app (from Terraform)": notes,
  "Orders service (patterns)": orders,
  "Reports service (VPC, zones)": reports,
};

export const exampleNames = Object.keys(examples);
