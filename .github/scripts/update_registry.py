#!/usr/bin/env python3
"""Update the release metadata for this plugin in registry.json."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def version_tuple(version: object) -> tuple[int, int, int] | None:
    if not isinstance(version, str):
        return None
    parts = version.split(".")
    if len(parts) != 3 or not all(part.isdigit() for part in parts):
        return None
    return tuple(int(part) for part in parts)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--path", required=True, type=Path)
    parser.add_argument("--plugin-id", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--darwin-sha256", required=True)
    parser.add_argument("--linux-sha256", required=True)
    args = parser.parse_args()

    registry = json.loads(args.path.read_text(encoding="utf-8"))
    plugin = next(
        (item for item in registry.get("plugins", []) if item.get("id") == args.plugin_id),
        None,
    )
    if plugin is None:
        raise SystemExit(f"plugin {args.plugin_id!r} not found in {args.path}")

    # workflow_run completions can arrive out of order when two releases are
    # built concurrently. Never let an older release roll the registry back.
    current_version = version_tuple(plugin.get("version"))
    release_version = version_tuple(args.version)
    if release_version is None:
        raise SystemExit(f"release version must be MAJOR.MINOR.PATCH: {args.version}")
    if current_version is not None and release_version < current_version:
        return

    plugin["version"] = args.version

    # Schema 1 uses the github-release installer, which derives the asset URL
    # from the version.  Keep that installer unchanged and only advance its
    # version.  Older schema 2 registries store fixed URLs and checksums, so
    # keep those artifact entries in sync as well.
    install = plugin.get("install")
    if isinstance(install, dict) and (
        install.get("type") == "direct" or "artifacts" in install
    ):
        repository = plugin.get("repository")
        if not repository:
            raise SystemExit(f"plugin {args.plugin_id!r} has no repository")

        base_url = f"{repository}/releases/download/v{args.version}"
        install["type"] = "direct"
        install["artifacts"] = [
            {
                "goos": "darwin",
                "goarch": "arm64",
                "url": f"{base_url}/{args.plugin_id}_{args.version}_darwin_arm64.zip",
                "sha256": args.darwin_sha256,
            },
            {
                "goos": "linux",
                "goarch": "amd64",
                "url": f"{base_url}/{args.plugin_id}_{args.version}_linux_amd64.zip",
                "sha256": args.linux_sha256,
            },
        ]

    args.path.write_text(
        json.dumps(registry, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )


if __name__ == "__main__":
    main()
