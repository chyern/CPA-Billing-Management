#!/usr/bin/env python3
"""Update the release metadata for this plugin in registry.json."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--path", required=True, type=Path)
    parser.add_argument("--plugin-id", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--darwin-sha256", required=True)
    parser.add_argument("--linux-sha256", required=True)
    args = parser.parse_args()

    registry = json.loads(args.path.read_text())
    plugin = next(
        (item for item in registry.get("plugins", []) if item.get("id") == args.plugin_id),
        None,
    )
    if plugin is None:
        raise SystemExit(f"plugin {args.plugin_id!r} not found in {args.path}")

    repository = plugin.get("repository")
    if not repository:
        raise SystemExit(f"plugin {args.plugin_id!r} has no repository")

    plugin["version"] = args.version
    base_url = f"{repository}/releases/download/v{args.version}"
    plugin["install"]["artifacts"] = [
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

    args.path.write_text(json.dumps(registry, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
