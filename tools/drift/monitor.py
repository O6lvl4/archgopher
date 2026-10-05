#!/usr/bin/env python3
"""Read upstream catalog sources and maintain reviewable, deduplicated findings.

No cloud resources are created. Publishing requires the explicit --publish switch.
Pricing documentation changes are review candidates, never automatic price edits.
"""
from __future__ import annotations

import argparse
import copy
import datetime as dt
import difflib
import hashlib
import html
from html.parser import HTMLParser
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed

SCHEMA = 1
BOT_LOGIN = "github-actions[bot]"
MAX_BYTES = 12 * 1024 * 1024
ALLOWED_HOSTS = frozenset({
    "api.github.com", "registry.terraform.io", "developers.cloudflare.com",
    "www.cloudflare.com", "aws.amazon.com", "azure.microsoft.com",
    "learn.microsoft.com", "cloud.google.com", "docs.cloud.google.com",
    "vps.conoha.jp", "www.conoha.jp",
})


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest()


def read_json(path, default=None):
    if not Path(path).exists():
        return copy.deepcopy(default)
    return json.loads(Path(path).read_text())


def save_json(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(data, ensure_ascii=False, sort_keys=True, indent=2) + "\n")
    temporary.replace(path)


def checked_url(url):
    p = urllib.parse.urlsplit(url)
    if p.scheme != "https" or p.hostname not in ALLOWED_HOSTS or p.username or p.password or p.port not in (None, 443):
        raise ValueError("URL is outside the explicit upstream allowlist")
    return url


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        checked_url(newurl)
        redirected = super().redirect_request(req, fp, code, msg, headers, newurl)
        # GitHub credentials must never follow a redirect to another origin.
        if urllib.parse.urlsplit(req.full_url).netloc != urllib.parse.urlsplit(newurl).netloc:
            redirected.remove_header("Authorization")
        return redirected


class HTTP:
    def __init__(self, token=None):
        self.token = token
        self.opener = urllib.request.build_opener(SafeRedirect())

    def request(self, url, data=None, method=None):
        checked_url(url)
        headers = {"User-Agent": "archgopher-catalog-drift/1.0", "Accept": "application/json, text/html, application/pdf;q=0.9"}
        if urllib.parse.urlsplit(url).hostname == "api.github.com" and self.token:
            headers["Authorization"] = "Bearer " + self.token
            headers["X-GitHub-Api-Version"] = "2022-11-28"
        body = None if data is None else json.dumps(data).encode()
        if body is not None:
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=body, headers=headers, method=method)
        for attempt in range(3):
            try:
                with self.opener.open(req, timeout=45) as response:
                    if response.status == 206:
                        raise ValueError("partial HTTP response; no complete snapshot")
                    payload = response.read(MAX_BYTES + 1)
                    expected = response.headers.get("Content-Length")
                    if expected is not None and int(expected) != len(payload):
                        raise ValueError("response length mismatch; no complete snapshot")
                    if len(payload) > MAX_BYTES:
                        raise ValueError("upstream response exceeds safety limit; not a complete snapshot")
                    return payload
            except urllib.error.HTTPError as exc:
                if exc.code not in (429, 500, 502, 503, 504) or attempt == 2 or data is not None:
                    raise RuntimeError(f"HTTP {exc.code} for {url}") from exc
                time.sleep(min(2 ** attempt, 4))
        raise RuntimeError("unreachable")

    def json(self, url):
        return json.loads(self.request(url))


def catalog_scope(root):
    """Only sources actually referenced by this checkout's public price books."""
    providers, pages = {}, {}
    for directory in sorted((Path(root) / "catalog").iterdir()):
        if not directory.is_dir():
            continue
        names = [p.parent.name for p in sorted(directory.glob("*/resource.yaml"))]
        if not names:
            continue
        free_path = directory / "free.txt"
        free = [x.strip() for x in free_path.read_text().splitlines() if x.strip() and not x.startswith("#")] if free_path.exists() else []
        providers[directory.name] = {"resources": names, "free": free}
        for path in sorted(directory.glob("*/books/prices.json")):
            for price_id, entry in json.loads(path.read_text()).items():
                # FX is intentionally separate from provider plan changes.
                if price_id.startswith("fx."):
                    continue
                url = urllib.parse.urldefrag(entry["source"])[0]
                checked_url(url)
                page = pages.setdefault(url, {"provider": directory.name, "files": set(), "prices": set(), "regions": set()})
                page["files"].add(str(path.relative_to(root)))
                page["prices"].add(price_id)
                page["regions"].update(entry.get("values", {}))
                for row in entry.get("rows", {}).values():
                    page["regions"].update(row)
    return providers, {u: {k: sorted(v) if isinstance(v, set) else v for k, v in p.items()} for u, p in pages.items()}


class PriceHTML(HTMLParser):
    """Keep content text and table order; drop chrome and executable content."""
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.all, self.main = [], []
        self.stack = []
        self.ignore = 0
        self.in_main = 0

    def handle_starttag(self, tag, attrs):
        ignored = tag in {"script", "style", "nav", "header", "footer", "noscript", "svg"}
        main = tag in {"main", "article"}
        self.stack.append((tag, ignored, main))
        self.ignore += ignored
        self.in_main += main
        if tag in {"p", "div", "tr", "li", "h1", "h2", "h3", "h4", "br"}:
            self.handle_data("\n")
        elif tag in {"td", "th"}:
            self.handle_data(" | ")
        if tag in {"br", "img", "meta", "link", "input", "hr", "source", "wbr"}:
            self.stack.pop()
            self.ignore -= ignored
            self.in_main -= main

    def handle_endtag(self, tag):
        for i in range(len(self.stack) - 1, -1, -1):
            if self.stack[i][0] == tag:
                for _, ignored, main in self.stack[i:]:
                    self.ignore -= ignored
                    self.in_main -= main
                del self.stack[i:]
                break
        if tag in {"p", "div", "tr", "li", "h1", "h2", "h3", "h4"}:
            self.handle_data("\n")

    def handle_data(self, text):
        if not self.ignore:
            self.all.append(text)
            if self.in_main:
                self.main.append(text)

    def text(self):
        return "".join(self.main or self.all)


def normalize_pricing_text(text):
    lines = []
    for line in text.splitlines():
        line = re.sub(r"\s+", " ", html.unescape(line)).strip()
        if not line or re.match(r"^(Last updated|最終更新|Was this helpful\?|Copyright|©)\b", line, re.I):
            continue
        lines.append(line)
    # Preserve headings, adjacent terms, table labels, currencies and units.
    # An amount found elsewhere is deliberately NOT considered an equivalent plan.
    return "\n".join(lines)


def pricing_snapshot(url, scope, fetch):
    body = fetch(url)
    if urllib.parse.urlsplit(url).path.lower().endswith(".pdf"):
        with tempfile.TemporaryDirectory(prefix="archgopher-prices-") as directory:
            pdf = Path(directory) / "source.pdf"
            pdf.write_bytes(body)
            result = subprocess.run(["pdftotext", "-layout", str(pdf), "-"], capture_output=True, check=True, timeout=45)
            text = result.stdout.decode("utf-8")
    else:
        raw = body.decode("utf-8", errors="strict")
        parser = PriceHTML()
        parser.feed(raw)
        text = parser.text()
    text = normalize_pricing_text(text)
    if len(text) < 120 or not re.search(r"(?:[$¥￥€]\s*\d|\d[\d,. ]*(?:円|USD|JPY|EUR)|\b(?:USD|JPY|EUR)\s*\d)", text, re.I):
        raise ValueError("no usable pricing content; empty, blocked or client-rendered response")
    if re.search(r"(access denied|just a moment|verify you are human|captcha)", text, re.I) and len(text) < 5000:
        raise ValueError("upstream access challenge; no snapshot accepted")
    return {"kind": "pricing-document", "provider": scope["provider"], "url": url,
            "scope": scope, "records": {"document": {"text": text}},
            "limitation": "Documentation drift is a review candidate, not proof of a changed billable rate. Only linked pages are checked; dynamic/API-only changes can be missed. AWS/Azure exact mapped rates remain covered by weekly sync; GCP needs credentials."}


def collect_prices(pages, fetch, workers=4):
    snapshots, errors = {}, {}
    with ThreadPoolExecutor(max_workers=workers) as pool:
        future_map = {pool.submit(pricing_snapshot, u, s, fetch): (u, s) for u, s in pages.items()}
        for future in as_completed(future_map):
            url, scope = future_map[future]
            key = "pricing:" + digest(url)[:24]
            try:
                snapshots[key] = future.result()
            except Exception as exc:
                errors[key] = {"kind": "pricing-document", "provider": scope["provider"], "url": url, "error": str(exc)[:400]}
    return snapshots, errors


def validate_snapshot(snapshot):
    if not isinstance(snapshot, dict) or not snapshot.get("records") or not isinstance(snapshot["records"], dict):
        raise ValueError("empty/incomplete snapshot")
    for field in ("kind", "provider", "url"):
        if not isinstance(snapshot.get(field), str) or not snapshot[field]:
            raise ValueError("snapshot lacks " + field)


def meaningful_record(record):
    if not isinstance(record, dict):
        return record
    if "classification" not in record:
        return record
    # Local coverage classification is review metadata, not upstream drift.
    result = {k: v for k, v in record.items() if k != "classification"}
    if record["classification"] == "unmodeled":
        result["schema"] = {}
    return result


def changes(old, new):
    before, after = old["records"], new["records"]
    changed = []
    for key in sorted(set(before) & set(after)):
        a, b = before[key], after[key]
        if isinstance(a, dict) and isinstance(b, dict) and a.get("classification") != b.get("classification"):
            continue
        if meaningful_record(a) != meaningful_record(b):
            changed.append(key)
    return {"added": sorted(set(after) - set(before)), "removed": sorted(set(before) - set(after)), "changed": changed}


def advance(state, snapshots, errors, now):
    """Successful sources advance independently; failed fetches never erase data."""
    if state is None:
        state = {"schema": SCHEMA, "sources": {}, "findings": {}}
    if state.get("schema") != SCHEMA or not isinstance(state.get("sources"), dict) or not isinstance(state.get("findings"), dict):
        raise ValueError("unsupported/corrupt baseline; refusing a silent reset")
    state = copy.deepcopy(state)
    snapshots, errors = dict(snapshots), dict(errors)
    for key, snapshot in list(snapshots.items()):
        prior = state["sources"].get(key)
        if prior and snapshot.get("kind") == "pricing-document":
            before = prior["snapshot"]["records"]["document"]["text"]
            after = snapshot["records"]["document"]["text"]
            amounts = r"(?:[$¥￥€]\s*\d|\b(?:USD|JPY|EUR)\s*\d|\d[\d,. ]*(?:円|USD|JPY|EUR))"
            old_prices = len(re.findall(amounts, before, re.I))
            new_prices = len(re.findall(amounts, after, re.I))
            if (len(before) >= 1000 and len(after) < len(before) * 0.6) or (old_prices >= 10 and new_prices < old_prices * 0.5):
                errors[key] = {"kind": snapshot["kind"], "provider": snapshot["provider"], "url": snapshot["url"],
                               "error": "Possible incomplete pricing page: substantial content or price-row loss; last good snapshot retained. Review the source before an intentional rebaseline."}
                del snapshots[key]
    for key, snapshot in sorted(snapshots.items()):
        validate_snapshot(snapshot)
        prior = state["sources"].get(key)
        if prior and any(changes(prior["snapshot"], snapshot).values()):
            pending = state["findings"].get(key)
            before = pending["before"] if pending and "before" in pending else prior["snapshot"]
            delta = changes(before, snapshot)
            state["findings"][key] = {"subject": key, "fingerprint": digest({k: meaningful_record(v) for k, v in snapshot["records"].items()}),
                "snapshot": snapshot, "before": before, "delta": delta, "observedAt": now,
                "reverted": not any(delta.values())}
        state["sources"][key] = {"snapshot": snapshot, "lastSuccessfulCheck": now}
    # One health Issue, even when many sources fail on the first run.
    health = "monitor:source-health"
    if errors:
        state["findings"][health] = {"subject": health, "fingerprint": digest(errors), "errors": errors, "observedAt": now}
    elif health in state["findings"] and state["findings"][health].get("errors"):
        state["findings"][health] = {"subject": health, "fingerprint": digest({"recovered": True}), "recovered": True, "observedAt": now}
    state["lastRun"] = now
    state["health"] = {"successful": len(snapshots), "failed": len(errors), "errors": errors}
    return state


def diff_excerpt(finding):
    old, new = finding["before"], finding["snapshot"]
    if new["kind"] == "pricing-document":
        a = old["records"]["document"]["text"].splitlines()
        b = new["records"]["document"]["text"].splitlines()
        lines = list(difflib.unified_diff(a, b, fromfile="previous source", tofile="current source", n=2))
        text = "\n".join(lines[:120])
        return text[:12000] + ("\n[truncated; see the run artifact]" if len(lines) > 120 or len(text) > 12000 else "")
    summary = json.dumps(finding["delta"], ensure_ascii=False, indent=2)
    details = []
    for key in finding["delta"]["changed"][:5]:
        a = json.dumps(meaningful_record(old["records"][key]), ensure_ascii=False, sort_keys=True, indent=2).splitlines()
        b = json.dumps(meaningful_record(new["records"][key]), ensure_ascii=False, sort_keys=True, indent=2).splitlines()
        details.extend(difflib.unified_diff(a, b, fromfile=key + " before", tofile=key + " after", n=2))
    return (summary + "\n" + "\n".join(details))[:12000]


def issue_text(finding, run_url=""):
    subject = finding["subject"]
    if finding.get("errors"):
        title = "[catalog drift] Restore incomplete upstream checks"
        lines = ["Some upstream sources could not be checked. Their previous snapshots were retained; this is not a no-change result.", ""]
        for key, value in sorted(finding["errors"].items()):
            lines.append(f"- {key}: {value.get('url', '')}: {value['error']}")
        lines += ["", "Acceptance: restore access/parsing, rerun the monitor, and confirm each affected source has a successful snapshot. GCP API pricing still requires separately authorized credentials."]
    elif finding.get("recovered"):
        title = "[catalog drift] Upstream checks recovered"
        lines = ["All sources attempted in this run succeeded. Earlier missing checks have recovered. Review any outstanding findings separately."]
    elif finding.get("sync"):
        title = "[catalog drift] Review weekly price-sync coverage"
        lines = ["Weekly sync reported incomplete coverage or validation failure.", json.dumps(finding["sync"], sort_keys=True),
                 "Existing price rows are not proof of complete provider coverage. Resolve failed mappings and review missing credentials; rerun tests before accepting the price PR."]
    else:
        snap = finding["snapshot"]
        title = f"[catalog drift] Review {snap['provider']} {snap['kind']} change"
        lines = [f"Source: {snap['url']}", f"Observed: {finding['observedAt']}",
                 f"Version: {finding['before'].get('version', 'previous fetch')} → {snap.get('version', 'current fetch')}"]
        if snap.get("providerAddress"):
            lines += [f"Definition source: {snap['providerAddress']}@{snap['version']}",
                      f"Schema export: {snap['schemaCommand']} ({snap['schemaCollection']})"]
        if snap.get("scope"):
            lines += ["Affected book files: " + ", ".join(snap["scope"].get("files", [])),
                      "Modeled regions: " + ", ".join(snap["scope"].get("regions", []))]
        lines += ["", snap.get("limitation", "Schema support does not establish billability or archgopher coverage. Free/helper and unmodeled resources must be classified before implementation."),
                  "", "Detected difference:", "<pre>",
                  ("The source has returned to the original baseline. The previously reported difference is no longer observed; verify whether any local follow-up is still needed."
                   if finding.get("reverted") else html.escape(diff_excerpt(finding))), "</pre>", "", "Acceptance checklist:",
                  "- Verify the current official source and effective date; distinguish newly offered, retired and renamed resources",
                  "- Review affected resource.yaml paths, helper/free classifications, readings and book mappings",
                  "- Preserve region, SKU, unit/per, currency/tax, tiers/pool/included units, commitment/renewal term and discount validity",
                  "- Add cases reproducing the estimate or import change; run Go checks and affected UI build",
                  "- Link the existing prices/sync PR when the change is already there; otherwise prepare a reviewed draft PR"]
    if run_url:
        lines += ["", "Evidence and full snapshots: " + run_url]
    metadata = json.dumps({"subject": subject, "fingerprint": finding["fingerprint"]}, sort_keys=True)
    body = "<!-- archgopher-drift " + metadata + " -->\n<!-- archgopher-drift:start -->\n" + "\n".join(lines)[:48000] + "\n<!-- archgopher-drift:end -->"
    return title, body


META = re.compile(r"<!-- archgopher-drift (\{[^\n]*\}) -->")
BLOCK = re.compile(r"^<!-- archgopher-drift \{[^\n]*\} -->\n^<!-- archgopher-drift:start -->$.*?^<!-- archgopher-drift:end -->$", re.S | re.M)


def issue_metadata(issue):
    match = META.search(issue.get("body") or "")
    if not match:
        return None
    try:
        metadata = json.loads(match[1])
        if not isinstance(metadata, dict) or not isinstance(metadata.get("subject"), str) or not re.fullmatch(r"[a-f0-9]{64}", str(metadata.get("fingerprint", ""))):
            return None
        return metadata
    except json.JSONDecodeError:
        return None


def issue_actions(findings, issues, run_url=""):
    actions = []
    for finding in findings.values():
        matches = [i for i in issues if i.get("user", {}).get("login") == BOT_LOGIN and (issue_metadata(i) or {}).get("subject") == finding["subject"] and not i.get("pull_request")]
        if any(any((label.get("name") if isinstance(label, dict) else label) == "drift:ignore"
                   for label in i.get("labels", [])) for i in matches):
            continue  # Maintainer opt-out for this subject; remove the label to resume.
        opened = sorted((i for i in matches if i["state"] == "open"), key=lambda i: i["number"], reverse=True)
        title, body = issue_text(finding, run_url)
        if opened:
            issue = opened[0]
            if issue_metadata(issue).get("fingerprint") != finding["fingerprint"]:
                original = issue.get("body") or ""
                if not BLOCK.search(original):
                    actions.append({"action": "blocked", "number": issue["number"],
                                    "reason": "Bot marker was edited; restore it or use drift:ignore. Human text was preserved."})
                    continue
                actions.append({"action": "update", "number": issue["number"], "body": BLOCK.sub(lambda _: body, original, count=1)})
        elif any((issue_metadata(i) or {}).get("fingerprint") == finding["fingerprint"] for i in matches):
            continue  # Closed means acknowledged; identical evidence must not reopen it.
        elif not (finding.get("recovered") or finding.get("reverted")):
            if matches:
                body += f"\n\nNew evidence after earlier review: #{max(i['number'] for i in matches)}"
            actions.append({"action": "create", "title": title, "body": body})
    return actions


def list_issues(http, repository):
    issues = []
    for page in range(1, 101):
        batch = http.json(f"https://api.github.com/repos/{repository}/issues?state=all&per_page=100&page={page}")
        if not isinstance(batch, list):
            raise ValueError("incomplete Issue inventory; refusing publication")
        issues.extend(batch)
        if len(batch) < 100:
            return issues
    raise ValueError("Issue inventory exceeded page budget; refusing duplicate-prone publication")


def acknowledge_closed(state, issues):
    if state is None:
        return None
    state = copy.deepcopy(state)
    for issue in issues:
        metadata = issue_metadata(issue)
        if issue.get("user", {}).get("login") != BOT_LOGIN or issue.get("state") != "closed" or not metadata:
            continue
        pending = state.get("findings", {}).get(metadata["subject"])
        if pending and pending["fingerprint"] == metadata["fingerprint"]:
            del state["findings"][metadata["subject"]]
    return state


def publish(http, repository, findings, run_url="", issues=None):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("invalid repository")
    actions = issue_actions(findings, list_issues(http, repository) if issues is None else issues, run_url)
    for action in actions:
        if action["action"] == "blocked":
            continue
        url = f"https://api.github.com/repos/{repository}/issues"
        if action["action"] == "update":
            http.request(url + "/" + str(action["number"]), {"body": action["body"]}, "PATCH")
        else:
            http.request(url, {"title": action["title"], "body": action["body"]}, "POST")
    return actions


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=".")
    parser.add_argument("--state", default=".drift/state.json")
    parser.add_argument("--out", default=".drift/report.json")
    parser.add_argument("--cache", default=".drift/cache")
    parser.add_argument("--publish", action="store_true")
    parser.add_argument("--repository", default="O6lvl4/archgopher")
    parser.add_argument("--run-url", default="")
    parser.add_argument("--only", choices=["prices", "providers"])
    parser.add_argument("--sync-report")
    args = parser.parse_args()
    http = HTTP(os.environ.get("GH_TOKEN"))
    now = dt.datetime.now(dt.timezone.utc).isoformat()
    if args.sync_report:
        summary = read_json(args.sync_report)
        if not isinstance(summary, dict):
            raise ValueError("missing price sync summary")
        findings = {}
        if summary.get("failed") or summary.get("skipped") or summary.get("validationFailed"):
            findings["monitor:weekly-sync"] = {"subject": "monitor:weekly-sync", "fingerprint": digest(summary), "sync": summary, "observedAt": now}
        actions = publish(http, args.repository, findings, args.run_url) if args.publish else []
        save_json(args.out, {"findings": findings, "actions": actions})
        return
    scope, pages = catalog_scope(Path(args.root))
    snapshots, errors = {}, {}
    if args.only != "providers":
        snapshots, errors = collect_prices(pages, http.request)
    if args.only != "prices":
        import providers
        provider_snapshots, provider_errors = providers.collect(scope, Path(args.cache), http.json)
        snapshots.update(provider_snapshots)
        errors.update(provider_errors)
    previous = read_json(args.state)
    issues = list_issues(http, args.repository) if args.publish else None
    acknowledged = acknowledge_closed(previous, issues or [])
    state = advance(acknowledged, snapshots, errors, now)
    # Save pending findings before a mutation. An interrupted publish can be retried
    # by re-reading all Issues, without losing evidence or blindly repeating writes.
    save_json(args.state, state)
    actions = publish(http, args.repository, state["findings"], args.run_url, issues) if args.publish else issue_actions(state["findings"], [], args.run_url)
    save_json(args.out, {"dryRun": not args.publish, "baselineInitialized": previous is None,
        "successful": state["health"]["successful"], "failed": state["health"]["failed"], "errors": state["health"]["errors"],
        "findings": state["findings"], "actions": actions,
        "limitations": ["Pricing documents are change candidates, not machine-verified price updates.",
            "New products outside pages used by the current catalogs are not monitored.",
            "API-only SKU/region additions and private discounts may not appear in documentation.",
            "GCP exact-price sync still needs credentials; no credentials are provisioned here.",
            "ConoHa FX updates are deliberately separate from provider plan changes."]})
    print(json.dumps({"successful": state["health"]["successful"], "failed": state["health"]["failed"], "actions": len(actions), "dryRun": not args.publish}))
    if state["health"]["failed"] or any(action["action"] == "blocked" for action in actions):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
