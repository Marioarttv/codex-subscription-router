import json
from pathlib import Path
import sqlite3
import tempfile
import tomllib
import unittest

from isolate_codex_home import migrate_primary, normalize_notification, snapshot_home, share_primary_history


class HomeIsolationTests(unittest.TestCase):
    def test_shared_history_leaves_config_independent_and_shares_writer_locks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            official, router = root / "official", root / "router"
            for home in (official, router):
                (home / "sessions").mkdir(parents=True)
                (home / "config.toml").write_text(home.name)
            share_primary_history(router, official, root / "backups")
            (official / "sessions/new-chat.jsonl").write_text("new chat")
            self.assertEqual((router / "sessions/new-chat.jsonl").read_text(), "new chat")
            (router / "sessions/new-chat.jsonl").write_text("continued in router")
            self.assertEqual((official / "sessions/new-chat.jsonl").read_text(), "continued in router")
            self.assertEqual((router / "thread-writer-locks").resolve(), (official / "thread-writer-locks").resolve())
            self.assertEqual((official / "config.toml").read_text(), "official")
            self.assertEqual((router / "config.toml").read_text(), "router")
            share_primary_history(router, official, root / "backups")

    def test_notification_cleanup_preserves_custom_hook_and_is_idempotent(self):
        with tempfile.TemporaryDirectory() as temporary:
            config = Path(temporary) / "config.toml"
            custom = ["custom-notifier", "done"]
            nested = custom
            for _ in range(15):
                nested = ["/old/SkyComputerUseClient", "turn-ended", "--previous-notify", json.dumps(nested)]
            config.write_text("notify = " + json.dumps(nested) + '\nmodel = "test"\n')
            helper = Path("/new/SkyComputerUseClient")
            normalize_notification(config, helper)
            result = config.read_text()
            self.assertEqual(tomllib.loads(result), {
                "model": "test", "notify": [str(helper), "turn-ended", "--previous-notify", json.dumps(custom)]})
            normalize_notification(config, helper)
            self.assertEqual(config.read_text(), result)

    def test_snapshot_keeps_history_independent_and_includes_wal(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            original, copied = root / "official", root / "router"
            (original / "sessions").mkdir(parents=True)
            rollout = original / "sessions" / "task.jsonl"
            rollout.write_text("original\n")
            (original / "config.toml").write_text(
                'model = "test"\n[mcp_servers.tool.env]\nCODEX_HOME = '
                + json.dumps(str(original)) + "\n"
            )
            (original / "automations").mkdir()
            (original / "automations" / "job.toml").write_text('status = "ACTIVE"')
            db = sqlite3.connect(original / "state_5.sqlite")
            db.execute("PRAGMA journal_mode=WAL")
            db.execute("CREATE TABLE threads(id TEXT, rollout_path TEXT, cwd TEXT)")
            db.execute("INSERT INTO threads VALUES('task', ?, ?)", (str(rollout), str(root / 'project')))
            db.commit()
            self.assertTrue(snapshot_home(original, copied))
            with sqlite3.connect(copied / "state_5.sqlite") as snapshot:
                self.assertEqual(snapshot.execute("SELECT rollout_path,cwd FROM threads").fetchone(),
                                 (str(copied / "sessions" / "task.jsonl"), str(root / 'project')))
            (copied / "sessions" / "task.jsonl").write_text("router-only\n")
            self.assertEqual(rollout.read_text(), "original\n")
            self.assertNotEqual(rollout.stat().st_ino, (copied / "sessions" / "task.jsonl").stat().st_ino)
            self.assertIn(str(copied), (copied / "config.toml").read_text())
            self.assertFalse((copied / "automations").exists())
            self.assertFalse(snapshot_home(original, copied))
            self.assertEqual((copied / "sessions" / "task.jsonl").read_text(), "router-only\n")
            db.close()

    def test_migration_preserves_accounts_and_owners(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            original, state_root = root / "official", root / "mux"
            original.mkdir()
            state_root.mkdir()
            (original / "auth.json").write_text('{"test":"credential"}')
            state = {"accounts": [{"id": "primary", "codexHome": str(original)},
                                  {"id": "secondary", "codexHome": str(root / "secondary")}],
                     "threadOwner": {"task": "primary"}, "routingAccountId": "secondary"}
            (state_root / "state.json").write_text(json.dumps(state))
            destination = migrate_primary(state_root)
            migrated = json.loads((state_root / "state.json").read_text())
            self.assertEqual(migrated['accounts'][0]['codexHome'], str(destination))
            self.assertEqual(migrated['accounts'][1], state['accounts'][1])
            self.assertEqual(migrated['threadOwner'], state['threadOwner'])
            self.assertEqual(migrated['routingAccountId'], 'secondary')
            self.assertEqual((destination / 'auth.json').read_text(), (original / 'auth.json').read_text())
            self.assertEqual(json.loads((state_root / 'backups/state-before-home-isolation.json').read_text()), state)
            migrate_primary(state_root)
            self.assertEqual(json.loads((state_root / "state.json").read_text()), migrated)


if __name__ == "__main__":
    unittest.main()
