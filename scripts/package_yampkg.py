#!/usr/bin/env python3
"""Rebuild YAM v2 per-target .yampkg packages from a published base release.

The public tree builds the legacy universal .yap only.  YAM v2 packages carry a
Luna launcher and a seeded controls JSON on top of the same runtime.  This tool
patches those files in a published base package, replaces the supervisor and
recomputes the manifest hashes, so the result is reviewable and reproducible.

It never signs or publishes anything.  Use yam-publish separately.
"""
import argparse
import hashlib
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PLUGIN_REL = Path("files/share/luna/plugins/airplay.airplay-luna")
MANIFEST = "MANIFEST.yaml"
CONFIG_KEYS = (
    "voice",
    "native_volume",
    "ducking",
    "leds",
    "led_level",
    "native_player",
    "alice_button",
)
NEW_PERMISSIONS = (
    'permissions = ["exec", "clock", "led", "ipc.speech", "commands", '
    '"media.native", "input.alice"]'
)
OLD_PERMISSIONS = 'permissions = ["exec", "clock", "led", "ipc.speech", "commands"]'
LUNA_DEPENDENCY = "yam/luna >=0.2.24"


def sha256_bytes(data):
    return hashlib.sha256(data).hexdigest()


def sha256_file(path):
    return sha256_bytes(Path(path).read_bytes())


def load_matrix(path):
    data = json.loads(Path(path).read_text())
    if data.get("schema") != 1:
        raise SystemExit("unsupported targets matrix schema")
    return data


def validate_matrix(matrix):
    targets = matrix.get("targets")
    if not isinstance(targets, list) or not targets:
        raise SystemExit("targets matrix is empty")
    names = set()
    for target in targets:
        name = target.get("target", "")
        if not name or "/" in name or name in names:
            raise SystemExit("bad or duplicate target %r" % name)
        names.add(name)
        if not isinstance(target.get("controls_config"), bool):
            raise SystemExit("%s: controls_config must be a boolean" % name)
        if not target["controls_config"] and target.get("mango_guard"):
            raise SystemExit("%s: mango_guard is only for targets that run controls" % name)
        sleep = target.get("sleep")
        if not isinstance(sleep, list) or not sleep or not all(
            isinstance(v, int) and 0 < v <= 60 for v in sleep
        ):
            raise SystemExit("%s: sleep must be a list of 1..60 second values" % name)
        if not isinstance(target.get("dbus_export"), bool):
            raise SystemExit("%s: dbus_export must be a boolean" % name)
        if not isinstance(target.get("mango_guard"), bool):
            raise SystemExit("%s: mango_guard must be a boolean" % name)
        config = target.get("config")
        if not isinstance(config, dict) or set(config) != set(CONFIG_KEYS):
            raise SystemExit("%s: config must contain exactly %s" % (name, ", ".join(CONFIG_KEYS)))
        if not all(isinstance(config[key], bool) for key in CONFIG_KEYS if key != "led_level"):
            raise SystemExit("%s: config feature flags must be booleans" % name)
        if not isinstance(config["led_level"], int) or not 0 <= config["led_level"] <= 255:
            raise SystemExit("%s: led_level must be 0..255" % name)


def render_launcher(template, target):
    text = template
    text = text.replace(
        "@@MANGO_GUARD@@",
        ' && [ "$model" != "mango" ]' if target["mango_guard"] else "",
    )
    if target["dbus_export"]:
        text = text.replace(
            "@@DBUS_EXPORT@@\n",
            "export DBUS_SYSTEM_BUS_ADDRESS=unix:path=/run/dbus/system_bus_socket\n",
        )
    else:
        text = text.replace("@@DBUS_EXPORT@@\n", "")
    text = text.replace(
        "@@SLEEP_LINES@@\n", "".join("sleep %d\n" % value for value in target["sleep"])
    )
    text = text.replace(
        "@@CONTROLS_CONFIG@@",
        ' --controls-config "$config"' if target["controls_config"] else "",
    )
    if "@@" in text:
        raise SystemExit("%s: unrendered launcher placeholder" % target["target"])
    return text


def render_config(target):
    config = target["config"]
    ordered = {key: config[key] for key in CONFIG_KEYS}
    return json.dumps(ordered, indent=2) + "\n"


def check(matrix, template):
    for placeholder in (
        "@@MANGO_GUARD@@",
        "@@DBUS_EXPORT@@\n",
        "@@SLEEP_LINES@@\n",
        "@@CONTROLS_CONFIG@@",
    ):
        if placeholder not in template:
            raise SystemExit("launcher template lacks %s" % placeholder.strip("@\n"))
    for target in matrix["targets"]:
        launch = render_launcher(template, target)
        if not launch.startswith("#!/bin/sh\n"):
            raise SystemExit("%s: launcher must start with #!/bin/sh" % target["target"])
        if launch.count('exec "$here/bin/station-airplay"') != 1:
            raise SystemExit("%s: launcher must exec the supervisor once" % target["target"])
        if ('--controls-config "$config"' in launch) != target["controls_config"]:
            raise SystemExit("%s: launcher/config mismatch" % target["target"])
        json.loads(render_config(target))
    print("matrix OK: %d targets" % len(matrix["targets"]))


def _yaml():
    try:
        import yaml
    except ImportError:
        raise SystemExit(
            "PyYAML is required to build packages: "
            "python3 -m pip install -r release/yampkg/requirements.txt"
        )
    return yaml


def extract_package(package, destination):
    raw = subprocess.check_output(["zstd", "-dc", str(package)])
    with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
        for member in archive.getmembers():
            if not (member.isfile() or member.isdir()):
                raise SystemExit("%s: unsupported tar member %s" % (package, member.name))
            if member.name.startswith("/") or ".." in Path(member.name).parts:
                raise SystemExit("%s: unsafe tar member %s" % (package, member.name))
        try:
            archive.extractall(destination, filter="data")
        except TypeError:
            archive.extractall(destination)


def verify_base(stage, manifest):
    for relative, expected in manifest["files_sha256"].items():
        actual = sha256_file(Path(stage) / "files" / relative)
        if actual != expected:
            raise SystemExit("base package hash mismatch: %s" % relative)


def patch_plugin(path, base_version, version):
    text = Path(path).read_text()
    old = 'version = "%s"' % base_version
    if text.count(old) != 1:
        raise SystemExit("%s: expected one version line" % path)
    text = text.replace(old, 'version = "%s"' % version, 1)
    if '"media.native"' not in text or '"input.alice"' not in text:
        if text.count(OLD_PERMISSIONS) != 1:
            raise SystemExit("%s: cannot add narrow permissions" % path)
        text = text.replace(OLD_PERMISSIONS, NEW_PERMISSIONS, 1)
    Path(path).write_text(text)


def patch_manifest(manifest, version, stage):
    manifest["version"] = version
    depends = [item for item in manifest.get("depends", []) if not item.startswith("yam/luna")]
    depends.append(LUNA_DEPENDENCY)
    manifest["depends"] = depends
    manifest["files_sha256"] = {
        relative: sha256_file(Path(stage) / "files" / relative)
        for relative in manifest["files_sha256"]
    }
    for relative in (
        str(PLUGIN_REL / "launch-airplay.sh")[len("files/") :],
        str(PLUGIN_REL / "config/airplay-controls-v2.json")[len("files/") :],
    ):
        if relative not in manifest["files_sha256"]:
            raise SystemExit("base manifest lacks %s" % relative)


def verify_output(package, target, binary, version, launcher):
    yaml = _yaml()
    raw = subprocess.check_output(["zstd", "-dc", str(package)])
    with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
        manifest = yaml.safe_load(archive.extractfile(MANIFEST))
        if manifest["version"] != version or manifest["target"] != target["target"]:
            raise SystemExit("%s: manifest version/target mismatch" % package)
        if LUNA_DEPENDENCY not in manifest["depends"]:
            raise SystemExit("%s: manifest dependency missing" % package)
        for relative, expected in manifest["files_sha256"].items():
            actual = sha256_bytes(archive.extractfile("files/" + relative).read())
            if actual != expected:
                raise SystemExit("%s: output hash mismatch: %s" % (package, relative))
        payload = archive.extractfile(str(PLUGIN_REL / "bin/station-airplay")).read()
        if payload != binary:
            raise SystemExit("%s: supervisor payload mismatch" % package)
        if payload[:4] != b"\x7fELF" or payload[4] != 1:
            raise SystemExit("%s: supervisor is not ELF32" % package)
        if int.from_bytes(payload[18:20], "little") != 40:
            raise SystemExit("%s: supervisor is not ARM" % package)
        extracted = archive.extractfile(str(PLUGIN_REL / "launch-airplay.sh")).read().decode()
        if extracted != launcher:
            raise SystemExit("%s: launcher mismatch" % package)
        if ('--controls-config "$config"' in extracted) != target["controls_config"]:
            raise SystemExit("%s: launcher controls flag mismatch" % package)
        config = json.loads(
            archive.extractfile(str(PLUGIN_REL / "config/airplay-controls-v2.json")).read()
        )
        if config != target["config"]:
            raise SystemExit("%s: seeded config mismatch" % package)
        plugin = archive.extractfile(str(PLUGIN_REL / "plugin.toml")).read().decode()
        if 'version = "%s"' % version not in plugin:
            raise SystemExit("%s: plugin version mismatch" % package)


def find_base(base_dir, name):
    exact = Path(base_dir) / ("%s.yampkg" % name)
    if exact.is_file():
        return exact
    matches = sorted(Path(base_dir).glob("*-%s.yampkg" % name))
    if len(matches) != 1:
        raise SystemExit(
            "%s: expected exactly one base package, found %d" % (name, len(matches))
        )
    return matches[0]


def build_target(target, base_dir, binary, version, out_dir, force, template):
    base = find_base(base_dir, target["target"])
    stage = tempfile.mkdtemp(prefix="yampkg-%s-" % target["target"])
    try:
        extract_package(base, stage)
        manifest_path = Path(stage) / MANIFEST
        yaml = _yaml()
        manifest = yaml.safe_load(manifest_path.read_text())
        if manifest.get("target") != target["target"]:
            raise SystemExit("%s: base target mismatch" % base)
        base_version = manifest.get("version")
        if not base_version:
            raise SystemExit("%s: base version missing" % base)
        verify_base(stage, manifest)

        plugin = Path(stage) / PLUGIN_REL
        (plugin / "bin/station-airplay").write_bytes(binary)
        launcher = render_launcher(template, target)
        launcher_path = plugin / "launch-airplay.sh"
        launcher_path.write_text(launcher)
        os.chmod(launcher_path, 0o755)
        (plugin / "config/airplay-controls-v2.json").write_text(render_config(target))
        patch_plugin(plugin / "plugin.toml", base_version, version)
        patch_manifest(manifest, version, stage)
        manifest_path.write_text(
            yaml.safe_dump(manifest, sort_keys=False, allow_unicode=True)
        )

        out_dir.mkdir(parents=True, exist_ok=True)
        out = out_dir / ("station-airplay-%s-%s.yampkg" % (version, target["target"]))
        if out.exists() and not force:
            raise SystemExit("%s exists; pass --force to replace it" % out)
        subprocess.run(
            ["tar", "--zstd", "-cf", str(out), MANIFEST, "files"],
            cwd=stage,
            check=True,
        )
        verify_output(out, target, binary, version, launcher)
        return {
            "target": target["target"],
            "version": version,
            "controls_config": target["controls_config"],
            "package": out.name,
            "sha256": sha256_file(out),
            "size": out.stat().st_size,
            "base_sha256": sha256_file(base),
            "binary_sha256": sha256_bytes(binary),
        }
    finally:
        shutil.rmtree(stage)


def default_version():
    text = (ROOT / "plugin.toml").read_text()
    match = re.search(r'^version = "([^"]+)"', text, re.MULTILINE)
    if not match:
        raise SystemExit("cannot read version from plugin.toml")
    return match.group(1)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="validate the matrix only")
    parser.add_argument("--base", help="directory with published base .yampkg files")
    parser.add_argument("--binary", help="ARMv7 supervisor to embed")
    parser.add_argument("--version", help="output version (default: plugin.toml)")
    parser.add_argument("--out", default=str(ROOT / "dist/yampkg"))
    parser.add_argument("--matrix", default=str(ROOT / "release/yampkg/targets.json"))
    parser.add_argument(
        "--template", default=str(ROOT / "release/yampkg/launch-airplay.sh.in")
    )
    parser.add_argument("--targets", help="comma-separated subset of targets")
    parser.add_argument("--force", action="store_true", help="replace existing output")
    args = parser.parse_args(argv)

    matrix = load_matrix(args.matrix)
    validate_matrix(matrix)
    template = Path(args.template).read_text()
    check(matrix, template)
    if args.check:
        return 0
    if not args.base or not args.binary:
        parser.error("--base and --binary are required unless --check is used")
    if not Path(args.base).is_dir():
        parser.error("--base must be a directory")

    targets = matrix["targets"]
    if args.targets:
        wanted = {item.strip() for item in args.targets.split(",") if item.strip()}
        known = {target["target"] for target in targets}
        unknown = wanted - known
        if unknown:
            parser.error("unknown targets: %s" % ", ".join(sorted(unknown)))
        targets = [target for target in targets if target["target"] in wanted]

    binary = Path(args.binary).read_bytes()
    if binary[:4] != b"\x7fELF" or binary[4] != 1:
        raise SystemExit("supervisor must be ELF32")
    if int.from_bytes(binary[18:20], "little") != 40:
        raise SystemExit("supervisor must be ARM (EM_ARM=40)")
    version = args.version or default_version()
    if not re.fullmatch(r"[0-9A-Za-z][0-9A-Za-z._+-]*", version):
        parser.error("--version contains unsupported characters")
    out_dir = Path(args.out)
    records = []
    for target in targets:
        record = build_target(
            target, args.base, binary, version, out_dir, args.force, template
        )
        records.append(record)
        print(
            "%-16s %9d  %s"
            % (record["target"], record["size"], record["sha256"])
        )
    provenance = out_dir / ("provenance-%s.json" % version)
    provenance.write_text(
        json.dumps(
            {
                "version": version,
                "binary_sha256": sha256_bytes(binary),
                "packages": records,
            },
            indent=2,
        )
        + "\n"
    )
    print("built %d packages; provenance: %s" % (len(records), provenance))
    return 0


if __name__ == "__main__":
    sys.exit(main())
