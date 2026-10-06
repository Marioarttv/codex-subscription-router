"""Exercise the real launcher with a fake Electron entrypoint."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


@unittest.skipUnless(shutil.which('xcrun'), 'requires macOS compiler')
class LauncherTests(unittest.TestCase):
    def test_inherited_official_cli_cannot_bypass_router(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            macos = root / 'Router.app/Contents/MacOS'
            macos.mkdir(parents=True)
            launcher = macos / 'CodexSubscriptionRouterLauncher'
            subprocess.run(['xcrun', 'clang', '-Wall', '-Wextra', '-o', str(launcher),
                            str(Path(__file__).parent.parent / 'native/launcher.c')], check=True)
            electron = macos / 'ChatGPT'
            electron.write_text('#!/usr/bin/env python3\nimport os,json,sys\nprint(json.dumps({"cli":os.environ["CODEX_CLI_PATH"],"home":os.environ["CODEX_HOME"],"sqlite":os.environ["CODEX_SQLITE_HOME"],"args":sys.argv[1:]}))\n')
            electron.chmod(0o755)
            env = {**os.environ, 'CODEX_CLI_PATH': '/official/cli'}
            result = json.loads(subprocess.check_output([str(launcher), '--smoke'], env=env, text=True))
            self.assertEqual(Path(result['cli']).resolve(), root / 'Router.app/Contents/Resources/codex')
            self.assertEqual(Path(result['home']), Path.home() / '.codex-mux/primary/codex-home')
            self.assertEqual(Path(result['sqlite']), Path.home() / '.codex')
            self.assertEqual(result['args'][-1], '--smoke')
            self.assertTrue(result['args'][0].startswith('--user-data-dir='))


if __name__ == '__main__':
    unittest.main()
