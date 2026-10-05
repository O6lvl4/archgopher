#!/usr/bin/env python3
"""Read-only CI smoke test of real, checksum-locked upstream resource schemas."""
import argparse
import json
from pathlib import Path
import subprocess

import monitor
import providers


FIXTURES = Path(__file__).resolve().parent / "fixtures"


def verify(provider, root, output, run=subprocess.run):
    pin = monitor.read_json(FIXTURES / "provider-pins.json")[provider]
    source, _ = providers.REGISTRY[provider]
    if pin["source"] != source or not providers._STABLE.fullmatch(pin["version"]):
        raise ValueError("smoke pin does not match the fixed official provider source")
    lockfile = FIXTURES / f"{provider}.lock.hcl"
    lock = lockfile.read_text()
    if f'provider "registry.terraform.io/{source}"' not in lock or f'"{pin["version"]}"' not in lock:
        raise ValueError("provider lock does not match the pinned source/version")

    def locked_run(command, **kwargs):
        # Exercise the production export path and its sanitized environment.
        # The reviewed lock file is immutable; Terraform must verify the selected
        # official release against its pinned checksums before executing it.
        if command[1] == "init":
            (Path(kwargs["cwd"]) / ".terraform.lock.hcl").write_text(lock)
            command = [*command, "-lockfile=readonly"]
        return run(command, **kwargs)

    expected_url = f"https://registry.terraform.io/v1/providers/{source}/versions"

    def pinned_version(url):
        if url != expected_url:
            raise ValueError("unexpected Registry source requested")
        return {"versions": [{"version": pin["version"]}]}

    scope, _ = monitor.catalog_scope(Path(root))
    snapshots, errors = providers.collect({provider: scope[provider]}, None, pinned_version, locked_run)
    snapshot = snapshots.get(f"provider:{provider}")
    report = {"provider": provider, "source": source, "version": pin["version"],
              "checksums": str(lockfile.relative_to(Path(__file__).resolve().parent)),
              "success": snapshot is not None, "errors": errors}
    if snapshot:
        report.update(resourceCount=len(snapshot["records"]), coverage=snapshot["coverage"],
                      definitionSource=snapshot["providerAddress"],
                      normalizedSchemaDigest=monitor.digest(snapshot["records"]))
    monitor.save_json(output, report)
    print(json.dumps(report, sort_keys=True))
    return 0 if snapshot is not None else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("provider", choices=sorted(providers.REGISTRY))
    parser.add_argument("--root", default=".")
    parser.add_argument("--out", default=".drift/provider-smoke.json")
    args = parser.parse_args()
    raise SystemExit(verify(args.provider, args.root, args.out))


if __name__ == "__main__":
    main()
