"""Offline tests for provider selection, normalization, and failure isolation."""

import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

try:
    from . import providers
except ImportError:
    import providers


def schema_resource():
    return {"version": 3, "block": {"description": "Resource prose", "attributes": {
        "name": {"type": "string", "required": True, "description": "Name prose"},
        "description": {"type": "string", "optional": True},
        "metadata": {"type": ["object", {"description": "string"}], "computed": True},
        "settings": {"optional": True, "nested_type": {"nesting_mode": "list", "attributes": {
            "enabled": {"type": "bool", "required": True}}}},
    }, "block_types": {"rule": {"nesting_mode": "set", "min_items": 1, "max_items": 2,
                                "block": {"attributes": {"priority": {"type": "number", "optional": True}}}}}}}


def document(source="hashicorp/aws", names=("aws_instance", "aws_iam_role", "aws_new_thing")):
    return {"format_version": "1.0", "provider_schemas": {
        f"registry.terraform.io/{source}": {"provider": {"version": 0, "block": {}},
                                           "resource_schemas": {name: schema_resource() for name in names}}}}


class FakeTerraform:
    def __init__(self, output=None, fail=None):
        self.output = output if output is not None else document()
        self.calls = []
        self.fail = fail

    def __call__(self, command, **kwargs):
        root = Path(kwargs["cwd"])
        self.calls.append({"command": command, "config": json.loads((root / "main.tf.json").read_text()),
                           "rc": (root / "terraform.rc").read_text(), **kwargs})
        if command[1] == self.fail:
            raise subprocess.CalledProcessError(1, command, stderr="failure")
        return subprocess.CompletedProcess(command, 0, stdout=json.dumps(self.output), stderr="")


class FakeRegistry:
    def __init__(self, versions=None):
        self.versions = versions or ["1.9.0", "1.10.0", "2.0.0-beta.1", "1.2.100"]
        self.calls = []

    def __call__(self, url):
        self.calls.append(url)
        return {"versions": [{"version": version} for version in self.versions]}


class ProviderTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.scope = {"aws": {"resources": ["aws_instance"], "free": ["aws_iam_role"]}}
        self.fetch = FakeRegistry()
        self.run = FakeTerraform()

    def collect(self, scope=None, fetch=None, run=None):
        return providers.collect(self.scope if scope is None else scope, self.temp.name,
                                 self.fetch if fetch is None else fetch, self.run if run is None else run)

    def test_version_selection_classification_and_no_provider_configuration(self):
        with patch.dict(os.environ, {"AWS_SECRET_ACCESS_KEY": "do-not-inherit", "TF_CLI_ARGS": "-bad",
                                     "TF_REATTACH_PROVIDERS": "do-not-inherit"}):
            snapshots, errors = self.collect()
        self.assertEqual(errors, {})
        snapshot = snapshots["provider:aws"]
        self.assertEqual(snapshot["version"], "1.10.0")
        self.assertEqual(snapshot["records"]["aws_instance"]["schema"]["version"], 3)
        self.assertEqual({key: value["classification"] for key, value in snapshot["records"].items()},
                         {"aws_instance": "modeled", "aws_iam_role": "free", "aws_new_thing": "unmodeled"})
        self.assertEqual(self.fetch.calls, ["https://registry.terraform.io/v1/providers/hashicorp/aws/versions"])
        self.assertEqual([call["command"] for call in self.run.calls], [
            ["terraform", "init", "-backend=false", "-input=false", "-no-color"],
            ["terraform", "providers", "schema", "-json"]])
        for call in self.run.calls:
            self.assertEqual(call["config"], {"terraform": {"required_providers": {
                "aws": {"source": "hashicorp/aws", "version": "= 1.10.0"}}}})
            self.assertNotIn("AWS_SECRET_ACCESS_KEY", call["env"])
            self.assertNotIn("TF_CLI_ARGS", call["env"])
            self.assertNotIn("TF_REATTACH_PROVIDERS", call["env"])
            self.assertEqual(call["env"]["HOME"], call["cwd"])
            self.assertTrue(call["check"])
            self.assertGreater(call["timeout"], 0)
        self.assertFalse(Path(self.run.calls[0]["cwd"]).exists())

    def test_stable_semver_is_numeric_and_excludes_prereleases(self):
        self.assertEqual(providers._latest_version({"versions": [{"version": v} for v in
            ["9.99.0", "10.1.0", "10.10.0", "11.0.0-rc.1", "10.2.0"]]}), "10.10.0")
        for payload in ({}, {"versions": []}, {"versions": [{"version": "2.0.0-rc.1"}]},
                        {"versions": [{"version": 123}]}):
            with self.subTest(payload=payload), self.assertRaises(ValueError):
                providers._latest_version(payload)

    def test_current_provider_version_reuses_cache_and_reclassifies(self):
        before, errors = self.collect()
        self.assertFalse(errors)
        self.scope["aws"]["resources"].append("aws_new_thing")
        after, errors = self.collect()
        self.assertFalse(errors)
        self.assertEqual(len(self.run.calls), 2)
        self.assertEqual(len(self.fetch.calls), 2)
        self.assertEqual(after["provider:aws"]["records"]["aws_new_thing"]["classification"], "modeled")
        self.assertEqual(before["provider:aws"]["records"]["aws_new_thing"]["classification"], "unmodeled")
        self.fetch.versions = ["1.11.0"]
        self.collect()
        self.assertEqual(len(self.run.calls), 4)

    def test_corrupt_cache_is_refetched(self):
        self.collect()
        path = Path(self.temp.name) / "provider-aws.json"
        path.write_text("broken json")
        snapshots, errors = self.collect()
        self.assertFalse(errors)
        self.assertIn("provider:aws", snapshots)
        self.assertEqual(len(self.run.calls), 4)

    def test_registry_failure_does_not_return_stale_snapshot(self):
        self.collect()
        def broken_fetch(url):
            raise OSError("HTTP 503")
        snapshots, errors = self.collect(fetch=broken_fetch)
        self.assertEqual(snapshots, {})
        self.assertIn("HTTP 503", errors["provider:aws"]["error"])
        self.assertEqual(len(self.run.calls), 2)

    def test_schema_prose_noise_ignored_but_semantics_preserved(self):
        before = document()
        changed = copy.deepcopy(before)
        for schema in changed["provider_schemas"]["registry.terraform.io/hashicorp/aws"]["resource_schemas"].values():
            schema["block"]["description"] = "A prose-only rewrite"
            schema["block"]["description_kind"] = "markdown"
            schema["block"]["attributes"]["name"]["description"] = "Different wording"
            schema["block"]["attributes"]["name"]["computed"] = False
        normalize = lambda value: providers._schema_document(value, "registry.terraform.io/hashicorp/aws", "aws_")
        self.assertEqual(normalize(before), normalize(changed))
        base = normalize(before)["aws_instance"]
        self.assertIn("description", base["block"]["attributes"])
        self.assertEqual(base["block"]["attributes"]["metadata"]["type"], ["object", {"description": "string"}])
        mutations = [
            lambda s: s["block"]["attributes"]["name"].update(type="number"),
            lambda s: s["block"]["attributes"]["name"].update(required=False, optional=True),
            lambda s: s["block"]["attributes"]["name"].update(deprecated=True),
            lambda s: s["block"]["attributes"]["metadata"].update(computed=False),
            lambda s: s["block"]["attributes"]["settings"]["nested_type"].update(nesting_mode="set"),
            lambda s: s["block"]["block_types"]["rule"].update(max_items=4),
            lambda s: s.update(version=4),
        ]
        for mutation in mutations:
            other = copy.deepcopy(before)
            mutation(other["provider_schemas"]["registry.terraform.io/hashicorp/aws"]["resource_schemas"]["aws_instance"])
            self.assertNotEqual(normalize(before), normalize(other))

    def test_invalid_and_incomplete_sources_never_produce_snapshots_or_cache(self):
        invalid = [
            {"format_version": "2.0", "provider_schemas": {}},
            document(source="someone/aws"),
            document(names=()),
            document(names=("google_compute_instance",)),
            document(names=("aws_unrelated",)),
            {"format_version": "1.0", "provider_schemas": {"registry.terraform.io/hashicorp/aws": {
                "resource_schemas": {"aws_instance": {"version": 0}}}}},
        ]
        for payload in invalid:
            with self.subTest(payload=payload):
                snapshots, errors = self.collect(run=FakeTerraform(payload))
                self.assertEqual(snapshots, {})
                self.assertIn("provider:aws", errors)
                self.assertFalse((Path(self.temp.name) / "provider-aws.json").exists())
        for command in ("init", "providers"):
            snapshots, errors = self.collect(run=FakeTerraform(fail=command))
            self.assertEqual(snapshots, {})
            self.assertIn("failed (exit 1): failure", errors["provider:aws"]["error"])

    def test_missing_individual_type_and_external_catalog_type_do_not_fail(self):
        self.scope["aws"]["resources"] += ["aws_retired", "external_model"]
        snapshots, errors = self.collect()
        self.assertFalse(errors)
        self.assertEqual(snapshots["provider:aws"]["coverage"],
                         {"known_types": 3, "present": 2, "missing": ["aws_retired"]})
        self.assertNotIn("aws_retired", snapshots["provider:aws"]["records"])

    def test_only_selected_providers_are_fetched_and_errors_are_isolated(self):
        self.assertEqual(self.collect(scope={}), ({}, {}))
        self.assertEqual(self.fetch.calls, [])
        self.assertEqual(self.run.calls, [])
        def fetch(url):
            if "hashicorp/google" in url:
                raise ValueError("unavailable")
            return self.fetch(url)
        snapshots, errors = self.collect(scope={**self.scope, "gcp": {"resources": [], "free": []}}, fetch=fetch)
        self.assertEqual(set(snapshots), {"provider:aws"})
        self.assertEqual(set(errors), {"provider:gcp"})
        self.assertEqual(len(self.run.calls), 2)

    def test_http_reader_runtime_error_is_a_per_source_failure(self):
        def fetch(url):
            if "hashicorp/azurerm" in url:
                raise RuntimeError("HTTP 429: rate limited")
            return self.fetch(url)
        snapshots, errors = self.collect(scope={**self.scope, "azure": {"resources": [], "free": []}}, fetch=fetch)
        self.assertEqual(set(snapshots), {"provider:aws"})
        self.assertEqual(set(errors), {"provider:azure"})
        self.assertIn("HTTP 429", errors["provider:azure"]["error"])

    def test_all_registry_mappings_use_expected_official_source(self):
        for provider, (source, prefix) in providers.REGISTRY.items():
            with self.subTest(provider=provider):
                name = prefix + "example"
                snapshots, errors = self.collect(scope={provider: {"resources": [name], "free": []}},
                    run=FakeTerraform(document(source, (name,))))
                self.assertFalse(errors)
                self.assertEqual(set(snapshots), {f"provider:{provider}"})
                self.assertIn(f"/providers/{source}/", snapshots[f"provider:{provider}"]["url"])


class ConohaTests(unittest.TestCase):
    COMMIT = "a" * 40
    TREE = "b" * 40
    BLOB = "c" * 40

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.calls = []
        self.scope = {"conoha": {"resources": ["conohavps_instance"], "free": ["conohavps_keypair"]}}
        self.tree = {"sha": self.TREE, "truncated": False, "tree": [
            {"path": path, "type": "blob", "sha": self.BLOB} for path in (
                "docs/resources/instance.md", "docs/resources/keypair.md", "docs/resources/new_resource.md",
                "internal/provider/resource/instance.go", "docs/data-sources/flavor.md",
                "internal/provider/resource/lb_common.go", "README.md")]}

    def fetch(self, url):
        self.calls.append(url)
        if url == f"{providers.CONOHA_API}/commits/HEAD":
            return {"sha": self.COMMIT, "commit": {"tree": {"sha": self.TREE}}}
        if url == f"{providers.CONOHA_API}/git/trees/{self.TREE}?recursive=1":
            return self.tree
        raise AssertionError(f"unexpected fetch: {url}")

    def collect(self):
        def never_execute(*args, **kwargs):
            raise AssertionError("ConoHa fork must not be built or executed")
        return providers.collect(self.scope, self.temp.name, self.fetch, run=never_execute)

    def test_fork_commit_and_docs_only_without_execution_or_upstream_substitution(self):
        snapshots, errors = self.collect()
        self.assertFalse(errors)
        snapshot = snapshots["provider:conoha"]
        self.assertEqual(snapshot["kind"], "terraform-docs")
        self.assertEqual(snapshot["version"], self.COMMIT)
        self.assertEqual(snapshot["url"], "https://github.com/Aid-On/terraform-provider-conohavps")
        self.assertIn("not an executed Terraform schema", snapshot["limitation"])
        self.assertEqual(set(snapshot["records"]), {"conohavps_instance", "conohavps_keypair", "conohavps_new_resource"})
        instance = snapshot["records"]["conohavps_instance"]
        self.assertEqual(instance["schema"], {})
        self.assertEqual(instance["classification"], "modeled")
        self.assertEqual(instance["files"], {"docs/resources/instance.md": self.BLOB,
                                             "internal/provider/resource/instance.go": self.BLOB})
        self.assertEqual(snapshot["records"]["conohavps_keypair"]["classification"], "free")
        self.assertEqual(snapshot["records"]["conohavps_new_resource"]["classification"], "unmodeled")
        second, errors = self.collect()
        self.assertFalse(errors)
        self.assertEqual(second, snapshots)
        self.assertEqual(len(self.calls), 3)  # Commit checked; unchanged tree reused.

    def test_truncated_or_wrong_tree_or_no_docs_are_unavailable(self):
        original = copy.deepcopy(self.tree)
        for mutation in ({"truncated": True}, {"sha": "d" * 40}, {"tree": []}, {"tree": [{}]}):
            with self.subTest(mutation=mutation):
                self.tree = {**original, **mutation}
                snapshots, errors = self.collect()
                self.assertEqual(snapshots, {})
                self.assertIn("provider:conoha", errors)
                self.assertFalse((Path(self.temp.name) / "provider-conoha.json").exists())

    def test_docs_and_source_hashes_change_while_unrelated_docs_do_not(self):
        before, errors = self.collect()
        self.assertFalse(errors)
        self.COMMIT = "d" * 40
        self.tree["tree"][-1]["sha"] = "e" * 40  # README is not a resource surface.
        after, errors = self.collect()
        self.assertFalse(errors)
        self.assertEqual(before["provider:conoha"]["records"], after["provider:conoha"]["records"])
        self.COMMIT = "f" * 40
        self.tree["tree"][0]["sha"] = "e" * 40
        changed, errors = self.collect()
        self.assertFalse(errors)
        self.assertNotEqual(before["provider:conoha"]["records"], changed["provider:conoha"]["records"])


if __name__ == "__main__":
    unittest.main()
