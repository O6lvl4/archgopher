"""Read provider surfaces without configuring providers or touching cloud state.

Registry providers use Terraform's schema protocol. The unpublished ConoHa fork
is inspected as a Git tree only; its file hashes are review signals, not schemas
or evidence that an unmodeled resource is billable.
"""

import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


REGISTRY = {
    "aws": ("hashicorp/aws", "aws_"),
    "azure": ("hashicorp/azurerm", "azurerm_"),
    "gcp": ("hashicorp/google", "google_"),
    "cloudflare": ("cloudflare/cloudflare", "cloudflare_"),
}
CONOHA_REPO = "Aid-On/terraform-provider-conohavps"
CONOHA_URL = f"https://github.com/{CONOHA_REPO}"
CONOHA_API = f"https://api.github.com/repos/{CONOHA_REPO}"
CONOHA_LIMITATION = (
    "ConoHa uses the Aid-On fork's resource documentation and source-file hashes, "
    "not an executed Terraform schema. File changes may be prose or implementation "
    "changes; documentation can lag resource registration. No billability is inferred."
)
_STABLE = re.compile(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?")
_SHA = re.compile(r"[0-9a-f]{40}(?:[0-9a-f]{24})?")
_NAME = re.compile(r"[a-z][a-z0-9_]*")
_FLAGS = ("required", "optional", "computed", "sensitive", "write_only")
_CACHE_FORMAT = 1


def _object(value, label):
    if not isinstance(value, dict):
        raise ValueError(f"{label} must be an object")
    return value


def _latest_version(document):
    versions = _object(document, "registry response").get("versions")
    if not isinstance(versions, list) or not versions:
        raise ValueError("registry returned no provider versions")
    stable = []
    for entry in versions:
        version = _object(entry, "registry version").get("version")
        if not isinstance(version, str):
            raise ValueError("registry version is missing its version string")
        match = _STABLE.fullmatch(version)
        if match:
            stable.append((tuple(int(x) for x in match.groups()), version))
    if not stable:
        raise ValueError("registry returned no stable semantic provider version")
    return max(stable)[1]


def _deprecation(value, result):
    # Some schema producers emit a message instead of a boolean. Preserve the
    # status, not prose edits to that message. Absent and false are equivalent.
    deprecated = value.get("deprecated", False)
    if not isinstance(deprecated, (bool, str)):
        raise ValueError("deprecated must be a boolean or message")
    if deprecated:
        result["deprecated"] = True


def _attributes(value):
    result = {}
    for name, attribute in sorted(_object(value, "attributes").items()):
        attribute = _object(attribute, f"attribute {name}")
        normalized = {}
        if "type" in attribute:
            if not isinstance(attribute["type"], (str, list)):
                raise ValueError(f"attribute {name} has an invalid type")
            # Type expressions contain object field names; do not strip keys
            # such as 'description' from the object type itself.
            normalized["type"] = attribute["type"]
        if "nested_type" in attribute:
            nested = _object(attribute["nested_type"], "nested_type")
            normalized["nested_type"] = {
                "nesting_mode": _nesting_mode(nested),
                "attributes": _attributes(nested.get("attributes")),
            }
        if not ("type" in normalized or "nested_type" in normalized):
            raise ValueError(f"attribute {name} has no type")
        for flag in _FLAGS:
            if flag in attribute and not isinstance(attribute[flag], bool):
                raise ValueError(f"attribute {name}: {flag} must be boolean")
            if attribute.get(flag):
                normalized[flag] = True
        _deprecation(attribute, normalized)
        result[name] = normalized
    return result


def _nesting_mode(value):
    mode = value.get("nesting_mode")
    if mode not in ("single", "group", "list", "set", "map"):
        raise ValueError("invalid or missing nesting_mode")
    return mode


def _block(value):
    value = _object(value, "schema block")
    result = {"attributes": _attributes(value.get("attributes", {}))}
    blocks = {}
    for name, nested in sorted(_object(value.get("block_types", {}), "block_types").items()):
        nested = _object(nested, f"nested block {name}")
        normalized = {"nesting_mode": _nesting_mode(nested), "block": _block(nested.get("block"))}
        for key in ("min_items", "max_items"):
            if key in nested:
                if type(nested[key]) is not int or nested[key] < 0:
                    raise ValueError(f"{key} must be a nonnegative integer")
                if nested[key]:
                    normalized[key] = nested[key]
        blocks[name] = normalized
    result["block_types"] = blocks
    _deprecation(value, result)
    return result


def _resources(value, prefix):
    value = _object(value, "resource_schemas")
    if not value:
        raise ValueError("provider returned an empty resource schema map")
    result = {}
    for name, schema in sorted(value.items()):
        if not isinstance(name, str) or not _NAME.fullmatch(name) or not name.startswith(prefix):
            raise ValueError(f"unexpected resource type: {name!r}")
        schema = _object(schema, f"resource schema {name}")
        version = schema.get("version")
        if type(version) is not int or version < 0:
            raise ValueError(f"invalid resource schema version for {name}")
        result[name] = {"version": version, "block": _block(schema.get("block"))}
    return result


def _schema_document(document, address, prefix):
    document = _object(document, "Terraform schema document")
    if not re.fullmatch(r"1\.\d+", str(document.get("format_version", ""))):
        raise ValueError("unsupported Terraform schema format_version (expected major 1)")
    schemas = _object(document.get("provider_schemas"), "provider_schemas")
    if set(schemas) != {address}:
        raise ValueError(f"schema did not contain exactly the expected provider {address}")
    provider = _object(schemas[address], "provider schema")
    return _resources(provider.get("resource_schemas"), prefix)


def _coverage(names, selection, prefix):
    # The catalog includes external/synthetic types, retired names, and aliases.
    # A missing individual type is a drift signal, not a failed collection.
    # Refuse a surface matching fewer than half of known provider-prefixed types;
    # that is too incomplete to safely produce a mass-deletion report.
    expected = {name for name in selection.get("resources", []) if name.startswith(prefix)}
    expected.update(name for name in selection.get("free", []) if name.startswith(prefix))
    present = expected.intersection(names)
    if expected and len(present) * 2 < len(expected):
        raise ValueError(f"catalog coverage sanity check failed: {len(present)}/{len(expected)} known types present")
    return {"known_types": len(expected), "present": len(present), "missing": sorted(expected.difference(names))}


def _classification(name, selection):
    if name in selection.get("resources", []):
        return "modeled"
    if name in selection.get("free", []):
        return "free"
    return "unmodeled"


def _cache_read(cache_dir, provider, identity, version):
    if cache_dir is None:
        return None
    try:
        value = json.loads((Path(cache_dir) / f"provider-{provider}.json").read_text())
        if (isinstance(value, dict) and value.get("cache_format") == _CACHE_FORMAT
                and value.get("identity") == identity and value.get("version") == version):
            return value.get("resources")
    except (OSError, ValueError):
        pass
    return None


def _cache_write(cache_dir, provider, identity, version, resources):
    if cache_dir is None:
        return
    # Cache availability must not turn a complete upstream observation into an
    # unavailable source. Interrupted writes cannot replace a prior good entry.
    temporary = None
    try:
        directory = Path(cache_dir)
        directory.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(mode="w", dir=directory, delete=False) as handle:
            temporary = Path(handle.name)
            json.dump({"cache_format": _CACHE_FORMAT, "identity": identity,
                       "version": version, "resources": resources}, handle, sort_keys=True)
        temporary.replace(directory / f"provider-{provider}.json")
    except OSError:
        pass
    finally:
        if temporary is not None:
            try:
                temporary.unlink(missing_ok=True)
            except OSError:
                pass


def _export_schema(provider_source, version, run):
    with tempfile.TemporaryDirectory(prefix="archgopher-provider-") as directory:
        root = Path(directory)
        local_name = provider_source.split("/")[-1]
        (root / "main.tf.json").write_text(json.dumps({"terraform": {"required_providers": {
            local_name: {"source": provider_source, "version": f"= {version}"}}}}))
        # Do not inherit cloud credentials, TF_CLI_ARGS, TF_REATTACH_PROVIDERS,
        # user CLI overrides, Terraform state, or the checkout's configuration.
        (root / "terraform.rc").write_text('provider_installation { direct {} }\n')
        env = {"PATH": os.environ.get("PATH", os.defpath), "HOME": directory,
               "TF_CLI_CONFIG_FILE": str(root / "terraform.rc"),
               "TF_DATA_DIR": str(root / ".terraform"), "TF_IN_AUTOMATION": "1",
               "TF_INPUT": "0", "CHECKPOINT_DISABLE": "1"}
        for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"):
            if key in os.environ:
                env[key] = os.environ[key]
        commands = (["terraform", "init", "-backend=false", "-input=false", "-no-color"],
                    ["terraform", "providers", "schema", "-json"])
        for command in commands:
            try:
                result = run(command, cwd=directory, env=env, capture_output=True,
                             text=True, check=True, timeout=600)
            except subprocess.CalledProcessError as error:
                diagnostic = (error.stderr or error.stdout or "no diagnostic output").strip()
                diagnostic = diagnostic.replace(directory, "<temporary provider directory>")
                raise RuntimeError(f"Terraform {command[1]} failed (exit {error.returncode}): {diagnostic[:3000]}") from error
        return json.loads(result.stdout)


def _registry_snapshot(provider, selection, cache_dir, fetch_json, run):
    source, prefix = REGISTRY[provider]
    address = f"registry.terraform.io/{source}"
    version = _latest_version(fetch_json(f"https://registry.terraform.io/v1/providers/{source}/versions"))
    resources = _cache_read(cache_dir, provider, address, version)
    if resources is not None:
        try:
            resources = _resources(resources, prefix)
            _coverage(resources, selection, prefix)
        except ValueError:
            resources = None
    if resources is None:
        resources = _schema_document(_export_schema(source, version, run), address, prefix)
    coverage = _coverage(resources, selection, prefix)
    _cache_write(cache_dir, provider, address, version, resources)
    return {"kind": "terraform-schema", "provider": provider,
            "url": f"https://registry.terraform.io/providers/{source}/{version}/docs",
            "version": version, "coverage": coverage,
            "providerAddress": address, "schemaCommand": "terraform providers schema -json",
            "schemaCollection": "resource_schemas",
            "records": {name: {"classification": _classification(name, selection), "schema": schema}
                        for name, schema in resources.items()}}


def _sha(value, label):
    if not isinstance(value, str) or not _SHA.fullmatch(value):
        raise ValueError(f"missing or invalid {label}")
    return value


def _conoha_files(value):
    value = _object(value, "ConoHa resource files")
    if not value:
        raise ValueError("ConoHa tree has no resource documentation")
    result = {}
    for name, files in sorted(value.items()):
        if not isinstance(name, str) or not _NAME.fullmatch(name) or not name.startswith("conohavps_"):
            raise ValueError("invalid ConoHa resource name")
        stem = name.removeprefix("conohavps_")
        files = _object(files, "ConoHa resource files")
        doc = f"docs/resources/{stem}.md"
        if doc not in files:
            raise ValueError(f"missing ConoHa documentation for {name}")
        allowed = {doc, f"internal/provider/resource/{stem}.go"}
        if not set(files).issubset(allowed):
            raise ValueError("unexpected ConoHa resource file")
        result[name] = {path: _sha(sha, "blob hash") for path, sha in sorted(files.items())}
    return result


def _conoha_snapshot(selection, cache_dir, fetch_json):
    commit = _object(fetch_json(f"{CONOHA_API}/commits/HEAD"), "ConoHa commit")
    version = _sha(commit.get("sha"), "ConoHa commit SHA")
    tree_sha = _sha(_object(_object(commit.get("commit"), "commit").get("tree"), "tree").get("sha"), "tree SHA")
    resources = _cache_read(cache_dir, "conoha", CONOHA_REPO, version)
    if resources is not None:
        try:
            resources = _conoha_files(resources)
            _coverage(resources, selection, "conohavps_")
        except ValueError:
            resources = None
    if resources is None:
        tree = _object(fetch_json(f"{CONOHA_API}/git/trees/{tree_sha}?recursive=1"), "ConoHa tree")
        if tree.get("truncated") is not False or tree.get("sha") != tree_sha:
            raise ValueError("ConoHa tree is truncated or does not match the commit")
        entries = tree.get("tree")
        if not isinstance(entries, list):
            raise ValueError("ConoHa tree entries are missing")
        blobs = {}
        for entry in entries:
            entry = _object(entry, "ConoHa tree entry")
            if entry.get("type") == "blob":
                path = entry.get("path")
                if not isinstance(path, str) or path in blobs:
                    raise ValueError("ConoHa tree has invalid or duplicate paths")
                blobs[path] = _sha(entry.get("sha"), "blob hash")
        resources = {}
        for path in sorted(blobs):
            match = re.fullmatch(r"docs/resources/([a-z][a-z0-9_]*)\.md", path)
            if match:
                stem = match.group(1)
                files = {path: blobs[path]}
                source = f"internal/provider/resource/{stem}.go"
                if source in blobs:
                    files[source] = blobs[source]
                resources[f"conohavps_{stem}"] = files
        resources = _conoha_files(resources)
    coverage = _coverage(resources, selection, "conohavps_")
    _cache_write(cache_dir, "conoha", CONOHA_REPO, version, resources)
    return {"kind": "terraform-docs", "provider": "conoha", "url": CONOHA_URL,
            "version": version, "limitation": CONOHA_LIMITATION, "coverage": coverage,
            "records": {name: {"classification": _classification(name, selection),
                               "schema": {}, "files": files} for name, files in resources.items()}}


def collect(scope, cache_dir, fetch_json, run=subprocess.run):
    """Return complete snapshots and per-source errors for scoped providers only.

    ``fetch_json(url)`` must raise on HTTP, parsing, or size-limit failures. A
    failed source has an error and no snapshot, including when a stale cache is
    available. Callers must preserve their last good baseline for that source.
    """
    snapshots, errors = {}, {}
    for provider, selection in sorted(scope.items()):
        source_id = f"provider:{provider}"
        kind = "terraform-docs" if provider == "conoha" else "terraform-schema"
        source = REGISTRY.get(provider, (None, None))[0]
        url = CONOHA_URL if provider == "conoha" else (
            f"https://registry.terraform.io/providers/{source}" if source else "")
        try:
            if provider == "conoha":
                snapshot = _conoha_snapshot(selection, cache_dir, fetch_json)
            elif provider in REGISTRY:
                snapshot = _registry_snapshot(provider, selection, cache_dir, fetch_json, run)
            else:
                raise ValueError(f"unsupported provider: {provider}")
            snapshots[source_id] = snapshot
        except (OSError, ValueError, TypeError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
            errors[source_id] = {"provider": provider, "kind": kind, "url": url,
                                 "error": f"{type(error).__name__}: {error}"}
    return snapshots, errors
