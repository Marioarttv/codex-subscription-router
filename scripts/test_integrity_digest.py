"""Regression tests for the signed Electron 154 startup integrity gate."""
import hashlib
import plistlib
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from patch_app import patch_integrity_dictionary_digest

SENTINEL = b'AGbevlPCksUGKNL8TSn7wGmJEuJsXb2A'
OLD = {'Resources/app.asar': {'algorithm': 'SHA256', 'hash': '1' * 64}}
NEW = {'Resources/app.asar': {'algorithm': 'SHA256', 'hash': '2' * 64}}


class IntegrityDigestTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.app = Path(self.tmp.name) / 'Router.app'
        (self.app / 'Contents').mkdir(parents=True)
        (self.app / 'Contents/Info.plist').write_bytes(plistlib.dumps({'ElectronAsarIntegrity': NEW}))
        self.framework = self.app / 'Contents/Frameworks/Codex Framework.framework'
        helpers = self.framework / 'Versions/Current/Helpers'
        helpers.mkdir(parents=True)
        for name in ['Codex (Renderer).app', 'Codex (GPU).app']:
            (helpers / name).mkdir()
        self.executable = self.framework / 'runtime'
        self.original_digest = hashlib.sha256(('Resources/app.asarSHA256' + '1' * 64).encode()).digest()
        self.original = b'prefix' + SENTINEL + b'\x01\x01' + self.original_digest + b'suffix'
        self.executable.write_bytes(self.original)
        self.main = patch('patch_app.bundle_main_executable', return_value=self.executable)
        self.main.start()
        self.addCleanup(self.main.stop)

    @patch('patch_app.sign_runtime_bundle')
    def test_updates_digest_and_signs_helpers_before_framework(self, sign):
        patch_integrity_dictionary_digest(self.app, OLD, 'same-team')
        expected = hashlib.sha256(('Resources/app.asarSHA256' + '2' * 64).encode()).digest()
        self.assertEqual(self.executable.read_bytes(), self.original.replace(self.original_digest, expected))
        paths = [call.args[0] for call in sign.call_args_list]
        self.assertEqual([p.name for p in paths], ['Codex (GPU).app', 'Codex (Renderer).app', 'Codex Framework.framework'])
        self.assertTrue(all(call.args[1] == 'same-team' for call in sign.call_args_list))

    @patch('patch_app.sign_runtime_bundle')
    def test_mismatched_source_digest_is_rejected_without_mutation(self, sign):
        with self.assertRaisesRegex(RuntimeError, 'does not match'):
            patch_integrity_dictionary_digest(self.app, NEW, 'same-team')
        self.assertEqual(self.executable.read_bytes(), self.original)
        sign.assert_not_called()

    @patch('patch_app.sign_runtime_bundle')
    def test_unknown_digest_version_is_rejected_without_mutation(self, sign):
        invalid = self.original.replace(SENTINEL + b'\x01\x01', SENTINEL + b'\x01\x02')
        self.executable.write_bytes(invalid)
        with self.assertRaisesRegex(RuntimeError, 'does not match'):
            patch_integrity_dictionary_digest(self.app, OLD, 'same-team')
        self.assertEqual(self.executable.read_bytes(), invalid)
        sign.assert_not_called()


if __name__ == '__main__':
    unittest.main()
