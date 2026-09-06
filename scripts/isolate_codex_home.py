#!/usr/bin/env python3
"""Snapshot primary Codex data without sharing mutable app configuration."""

from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import tomllib
import time


# Runtime locks, sockets, queues, browser registrations, and scheduled jobs must
# not be copied into another running application. Project checkouts stay where
# they are; a copied task still opens the same user-owned project.
DIRECTORIES = (
    "sessions", "archived_sessions", "attachments", "generated_images",
    "visualizations", "skills", "plugins", "memories", "rules", "vendor_imports",
)
FILES = (
    "config.toml", "auth.json", ".credentials.json", "AGENTS.md", "instructions.md",
    "keybindings.json", "config.json", "history.json", "history.jsonl",
    "session_index.jsonl", ".codex-global-state.json", ".personality_migration",
)


def normalize_notification(config_path: Path, helper: Path) -> None:
    """Collapse copied turn-ended wrappers while preserving a user's own hook."""
    if not config_path.is_file():
        return
    contents = config_path.read_text()
    parsed = tomllib.loads(contents)
    previous = parsed.get("notify")
    if previous is None:
        return
    while (isinstance(previous, list) and len(previous) >= 2
           and Path(previous[0]).name == "SkyComputerUseClient"
           and previous[1] == "turn-ended"):
        previous = (json.loads(previous[previous.index("--previous-notify") + 1])
                    if "--previous-notify" in previous else None)
    hook = [str(helper), "turn-ended"]
    if previous:
        hook.extend(["--previous-notify", json.dumps(previous)])
    # Desktop writes this as a single top-level TOML line. Reject an unknown
    # representation rather than dropping unrelated user configuration.
    lines = contents.splitlines(keepends=True)
    for index, line in enumerate(lines):
        if line.strip().startswith("notify ="):
            lines[index] = "notify = " + json.dumps(hook) + "\n"
            break
    else:
        raise RuntimeError("unrecognized notification configuration")
    updated = "".join(lines)
    expected = dict(parsed, notify=hook)
    if tomllib.loads(updated) != expected:
        raise RuntimeError("notification cleanup would change other settings")
    temporary = config_path.with_suffix(".notify-tmp")
    temporary.write_text(updated)
    temporary.chmod(config_path.stat().st_mode & 0o777)
    temporary.replace(config_path)


def pin_router_runtime(config_path: Path, router_app: Path, router_home: Path, helper: Path) -> None:
    """Repair legacy MCP paths copied from the official app's configuration."""
    if not config_path.exists():
        return
    content = config_path.read_text()
    section = ""
    resources = router_app / "Contents" / "Resources"
    values = {
        "command": str(resources / "cua_node/bin/node_repl"),
        "CODEX_CLI_PATH": str(resources / "codex"),
        "CODEX_HOME": str(router_home),
        "NODE_REPL_NODE_PATH": str(resources / "cua_node/bin/node"),
        "NODE_REPL_NODE_MODULE_DIRS": str(resources / "cua_node/lib/node_modules"),
        "NODE_REPL_TRUSTED_CODE_PATHS": str(router_home) + ":" + str(resources / "cua_node/lib/node_modules"),
        "SKY_CUA_SERVICE_PATH": str(helper),
    }
    result = []
    for line in content.splitlines(keepends=True):
        if line.strip().startswith("["):
            section = line.strip()
        if section in ("[mcp_servers.node_repl]", "[mcp_servers.node_repl.env]"):
            key = line.split("=", 1)[0].strip()
            if key in values:
                line = key + " = " + json.dumps(values[key]) + "\n"
        result.append(line)
    updated = "".join(result)
    tomllib.loads(updated)
    temporary = config_path.with_suffix(".runtime-tmp")
    temporary.write_text(updated)
    temporary.chmod(config_path.stat().st_mode & 0o777)
    temporary.replace(config_path)


def copy_file(source: str, destination: str) -> str:
    # APFS clones are independent on write; unlike hard links they cannot let
    # one app append to the other app's conversation files.
    if sys.platform == "darwin":
        result = subprocess.run(
            ["/bin/cp", "-c", "-p", source, destination],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        if result.returncode == 0:
            return destination
    return shutil.copy2(source, destination)


def snapshot_database(source: Path, destination: Path, old: Path, new: Path) -> None:
    with sqlite3.connect(source.as_uri() + "?mode=ro", uri=True) as reader:
        with sqlite3.connect(destination) as writer:
            reader.backup(writer)
            tables = {row[0] for row in writer.execute(
                "SELECT name FROM sqlite_master WHERE type='table'"
            )}
            # Rebase file indexes, never rewrite stored conversation contents.
            for table in ("threads", "rollout_migration_skipped_rollouts"):
                if table in tables:
                    writer.execute(
                        f'UPDATE "{table}" SET rollout_path = ? || substr(rollout_path, ?) '
                        'WHERE substr(rollout_path, 1, ?) = ?',
                        (str(new) + "/", len(str(old)) + 2,
                         len(str(old)) + 1, str(old) + "/"),
                    )
            # A snapshot must not launch a second copy of active background work.
            if "thread_goals" in tables:
                writer.execute("UPDATE thread_goals SET status='paused' WHERE status='active'")
            if "remote_control_enrollments" in tables:
                writer.execute("DELETE FROM remote_control_enrollments")
            writer.commit()
            if writer.execute("PRAGMA quick_check").fetchone()[0] != "ok":
                raise RuntimeError(f"invalid database snapshot: {source.name}")
    destination.chmod(0o600)


def snapshot_home(source: Path, destination: Path) -> bool:
    source, destination = source.absolute(), destination.absolute()
    if source.resolve() == destination.resolve() or source.resolve() in destination.resolve().parents:
        raise RuntimeError("router home must be separate from the source Codex home")
    if destination.exists():
        return False
    destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".home-snapshot-", dir=destination.parent) as temp:
        staged = Path(temp) / "codex-home"
        staged.mkdir(mode=0o700)
        # Capture database state before copying append-only session files.
        for database in source.glob("*.sqlite"):
            if database.name.startswith(("logs_", "queue_")):
                continue
            snapshot_database(database, staged / database.name, source, destination)
        for name in DIRECTORIES:
            if (source / name).is_dir():
                shutil.copytree(source / name, staged / name, symlinks=True, copy_function=copy_file)
        for name in FILES:
            if (source / name).is_file():
                copy_file(str(source / name), str(staged / name))
        for name in ("config.toml", ".codex-global-state.json"):
            path = staged / name
            if path.is_file():
                # Keep project checkouts in place, including existing worktrees.
                contents = path.read_text()
                for directory in (*DIRECTORIES, "computer-use", "node_repl", "tmp", "sqlite"):
                    contents = contents.replace(str(source / directory), str(destination / directory))
                contents = contents.replace(
                    'CODEX_HOME = ' + json.dumps(str(source)),
                    'CODEX_HOME = ' + json.dumps(str(destination)),
                )
                path.write_text(contents)
        # Symlinks to app-owned state would defeat the snapshot isolation.
        for path in staged.rglob("*"):
            if path.is_symlink():
                target = os.readlink(path)
                if target.startswith(str(source) + "/"):
                    path.unlink()
                    path.symlink_to(str(destination) + target[len(str(source)):])
        (staged / ".router-home-snapshot.json").write_text(json.dumps({
            "source": str(source), "scheduled_jobs_copied": False,
            "active_goals_paused": True,
        }) + "\n")
        staged.rename(destination)
    return True


def migrate_primary(state_root: Path) -> Path:
    """Called by the installer while router components are stopped."""
    destination = state_root / "primary" / "codex-home"
    state_path = state_root / "state.json"
    state = json.loads(state_path.read_text()) if state_path.exists() else None
    primary = next((a for a in state["accounts"] if a["id"] == "primary"), None) if state else None
    source = Path(primary["codexHome"]) if primary else Path.home() / ".codex"
    if source.resolve() != destination.resolve():
        snapshot_home(source, destination)
    if primary and primary["codexHome"] != str(destination):
        backup = state_root / "backups" / "state-before-home-isolation.json"
        backup.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        if not backup.exists():
            shutil.copy2(state_path, backup)
        primary["codexHome"] = str(destination)
        temporary = state_path.with_suffix(".isolation-tmp")
        temporary.write_text(json.dumps(state, indent=2) + "\n")
        temporary.chmod(0o600)
        temporary.replace(state_path)
    return destination


def share_primary_history(router_home: Path, official_home: Path, backup_root: Path) -> None:
    """Reconnect a pristine snapshot using shared rollouts and writer locks.

    CODEX_SQLITE_HOME selects the official database; config.toml, credentials,
    plugins, notifications, and the Electron profile remain app-specific.
    """
    names = ("sessions", "archived_sessions", "thread-writer-locks")
    if all((router_home / name).is_symlink()
           and (router_home / name).resolve() == (official_home / name).resolve()
           for name in names):
        return
    official_db, router_db = official_home / "state_5.sqlite", router_home / "state_5.sqlite"
    if official_db.exists() and router_db.exists():
        with sqlite3.connect(official_db.as_uri() + "?mode=ro", uri=True) as db:
            original = dict(db.execute("SELECT id, updated_at FROM threads"))
        with sqlite3.connect(router_db.as_uri() + "?mode=ro", uri=True) as db:
            newer = [row for row in db.execute("SELECT id, updated_at FROM threads")
                     if row[0] not in original or row[1] > original[row[0]]]
        if newer:
            raise RuntimeError("router primary has newer or unique chats; merge them before enabling shared history")
    backup = backup_root / ("history-before-sharing-" + time.strftime("%Y%m%d-%H%M%S"))
    backup.mkdir(mode=0o700, parents=True, exist_ok=False)
    for name in names:
        target, link = official_home / name, router_home / name
        target.mkdir(mode=0o700, parents=True, exist_ok=True)
        if link.is_symlink() and link.resolve() == target.resolve():
            continue
        if link.exists() or link.is_symlink():
            link.rename(backup / name)
        link.symlink_to(target, target_is_directory=True)
    (router_home / ".router-shared-history.json").write_text(json.dumps({
        "sqlite_home": str(official_home), "shared_directories": names,
        "previous_snapshot": str(backup),
    }) + "\n")


if __name__ == "__main__":
    print(migrate_primary(Path.home() / ".codex-mux"))
