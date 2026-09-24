// Checks the dependency rule of the UI, the same one the Go side follows:
// lib <- ui <- composites <- features <- app. A feature may use its own files
// and the layers below it, never another feature.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";

const src = resolve(import.meta.dirname, "../src");
const rank = { lib: 0, ui: 1, composites: 2, features: 3, app: 4 };

function* files(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* files(p);
    else if (/\.tsx?$/.test(name)) yield p;
  }
}

/** The layer of a file and, for features, which feature. */
function place(file) {
  const [layer, feature] = relative(src, file).split("/");
  return { layer, feature: layer === "features" ? feature : undefined };
}

function allowed(from, to) {
  if (!(from.layer in rank) || !(to.layer in rank)) return false;
  if (from.layer === "features" && to.layer === "features") return from.feature === to.feature;
  return rank[to.layer] <= rank[from.layer];
}

const problems = [];
for (const file of files(src)) {
  const text = readFileSync(file, "utf8");
  for (const [, spec] of text.matchAll(/from\s+["'](\.[^"']+)["']/g)) {
    const target = resolve(dirname(file), spec);
    if (!target.startsWith(src)) continue;
    const from = place(file);
    const to = place(target);
    if (!allowed(from, to)) problems.push(`${relative(src, file)} imports ${relative(src, target)}`);
  }
}
if (problems.length > 0) {
  console.error("UI dependency rule broken (lib <- ui <- composites <- features <- app):");
  for (const p of problems) console.error(`  ${p}`);
  process.exit(1);
}
console.log("UI layers ok");
