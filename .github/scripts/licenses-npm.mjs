// Fails if any npm dependency (prod or dev) has a license outside
// licenses.allow. An SPDX expression passes if every AND-part has at least
// one allowed OR-alternative; parentheses are ignored (enough for npm data).
import { readFileSync } from "node:fs";
import { join } from "node:path";

const allowed = new Set(
  readFileSync("licenses.allow", "utf8")
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#")),
);
const ours = /^@polka\//;

function licenseOf(path, entry) {
  let lic = entry.license;
  if (!lic) {
    try {
      lic = JSON.parse(readFileSync(join(path, "package.json"), "utf8")).license;
    } catch {
      /* not installed (optional for another platform) */
    }
  }
  return typeof lic === "object" && lic !== null ? lic.type : lic;
}

function ok(expr) {
  return expr
    .replace(/[()]/g, " ")
    .split(/\s+AND\s+/)
    .every((part) => part.split(/\s+OR\s+/).some((alt) => allowed.has(alt.trim())));
}

const lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
const bad = [];
for (const [path, entry] of Object.entries(lock.packages)) {
  if (!path.includes("node_modules/")) continue;
  const name = path.split("node_modules/").pop();
  if (ours.test(name) || entry.link) continue;
  const lic = licenseOf(path, entry);
  if (!lic || !ok(lic)) bad.push(`${name}@${entry.version}: ${lic ?? "UNKNOWN"}`);
}
if (bad.length) {
  console.error(`npm packages with licenses not in licenses.allow:\n  ${bad.join("\n  ")}`);
  process.exit(1);
}
console.log(`npm: ${Object.keys(lock.packages).length - 1} packages, all licenses allowed`);
