import contextlib
import importlib.util
import io
from pathlib import Path
import stat
import tempfile
import types
import unittest


script = Path(__file__).with_name("hub-credentials.py")
spec = importlib.util.spec_from_file_location("hub_credentials", script)
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)


class CredentialBridgeTest(unittest.TestCase):
    def test_code_is_displayed_but_secrets_are_stored_privately(self):
        with tempfile.TemporaryDirectory() as temporary:
            bridge.CONFIG_DIR = Path(temporary) / "agent-control-plane"
            bridge.CREDS = bridge.CONFIG_DIR / "credentials.json"
            bridge.PENDING = bridge.CONFIG_DIR / "pending.json"
            bridge.LOCK = bridge.CONFIG_DIR / ".lock"
            responses = [
                (200, b'{"verification_uri":"https://agent.gwinam.com/activate",'
                      b'"user_code":"ABCD-1234","device_code":"private-device-code",'
                      b'"expires_in":600,"interval":5}'),
                (200, b'{"access_token":"private-access","refresh_token":"private-refresh",'
                      b'"expires_in":900}'),
            ]
            original_form = bridge.form
            bridge.form = lambda _path, _values: responses.pop(0)
            try:
                output = io.StringIO()
                with contextlib.redirect_stdout(output):
                    bridge.start(types.SimpleNamespace(label="Claude", scope="context:read"))
                    bridge.finish(None)
            finally:
                bridge.form = original_form
            self.assertIn("ABCD-1234", output.getvalue())
            self.assertNotIn("private-device-code", output.getvalue())
            self.assertNotIn("private-access", output.getvalue())
            self.assertNotIn("private-refresh", output.getvalue())
            self.assertFalse(bridge.PENDING.exists())
            self.assertEqual(bridge.read_private(bridge.CREDS)["refresh_token"], "private-refresh")
            self.assertEqual(stat.S_IMODE(bridge.CREDS.stat().st_mode), 0o600)
            self.assertEqual(stat.S_IMODE(bridge.CONFIG_DIR.stat().st_mode), 0o700)


if __name__ == "__main__":
    unittest.main()
