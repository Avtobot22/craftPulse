import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch
from urllib.error import HTTPError, URLError


ROOT = Path(__file__).resolve().parents[1]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


jenkins = load('jenkins90_acceptance', 'deploy/jenkins/acceptance.py')
sonar = load('sonar90_manage', 'deploy/sonarqube/manage.py')


class CISourceTransportTests(unittest.TestCase):
    def test_credentials_reject_insecure_or_ambiguous_origins(self):
        for module in (jenkins, sonar):
            for url in ('http://source.test', 'https://user:pass@source.test',
                        'https://source.test/path', 'https://source.test?token=secret',
                        'https://source.test/#fragment'):
                with self.subTest(module=module.__name__, url=url):
                    with self.assertRaises(ValueError):
                        module.API(url, 'reader', 'secret')

    def test_redirect_does_not_forward_credentials(self):
        for module in (jenkins, sonar):
            self.assertIsNone(module.NoRedirect().redirect_request(
                None, None, 302, 'redirect', {}, 'https://another.test'))

    def test_errors_do_not_expose_response_body_or_tokens(self):
        for module in (jenkins, sonar):
            client = module.API('https://source.test', 'reader', 'private-token')
            client.opener = Mock()
            client.opener.open.side_effect = HTTPError(
                'https://source.test', 403, 'forbidden', {}, io.BytesIO(b'private-token'))
            with self.assertRaises(RuntimeError) as failure:
                client.request('GET', '/api/check')
            self.assertNotIn('private-token', str(failure.exception))
            self.assertIn('403', str(failure.exception))

    def test_failed_certificate_is_not_accepted(self):
        for module in (jenkins, sonar):
            client = module.API('https://source.test', 'reader', 'private-token')
            client.opener = Mock()
            client.opener.open.side_effect = URLError('CERTIFICATE_VERIFY_FAILED private-token')
            with self.assertRaisesRegex(RuntimeError, 'TLS verification failed'):
                client.request('GET', '/api/check')
            self.assertEqual(client.opener.open.call_count, 1)


class SonarProvisioningTests(unittest.TestCase):
    def test_wrong_version_prevents_mutations(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'sonar-admin-password').write_text('private-password')
            client = Mock()
            client.request.side_effect = [(200, {'valid': True}), (200, 'wrong-version')]
            with patch.object(sonar, 'API', return_value=client):
                with self.assertRaisesRegex(RuntimeError, 'version'):
                    sonar.bootstrap('https://source.test', directory)
            self.assertEqual(client.request.call_args_list, [
                unittest.mock.call('GET', '/api/authentication/validate'),
                unittest.mock.call('GET', '/api/server/version')])

    def test_first_bootstrap_checks_version_with_default_credentials_before_password_change(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'sonar-admin-password').write_text('new-password')
            admin, initial = Mock(), Mock()
            admin.request.side_effect = [(200, {'valid': False}), RuntimeError('stop after password change')]
            initial.request.side_effect = [(200, sonar.VERSION), (204, None)]
            with patch.object(sonar, 'API', side_effect=(admin, initial)):
                with self.assertRaisesRegex(RuntimeError, 'stop after password change'):
                    sonar.bootstrap('https://source.test', directory)
            self.assertEqual(initial.request.call_args_list, [
                unittest.mock.call('GET', '/api/server/version'),
                unittest.mock.call('POST', '/api/users/change_password', {
                    'login': 'admin', 'previousPassword': 'admin', 'password': 'new-password'})])

    def test_state_from_another_instance_prevents_network_requests(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'sonar-credentials.json').write_text(
                json.dumps({'url': 'https://another.test', 'reader_token': 'private-token'}))
            with patch.object(sonar, 'API') as api:
                with self.assertRaisesRegex(ValueError, 'another origin'):
                    sonar.bootstrap('https://source.test', directory)
            api.assert_not_called()

    def test_private_credentials_cannot_be_written_into_checkout(self):
        with self.assertRaisesRegex(ValueError, 'outside the source checkout'):
            sonar.private_json(ROOT / 'deploy/sonarqube/credentials.json', {'token': 'private'})
        self.assertFalse((ROOT / 'deploy/sonarqube/credentials.json').exists())

    def test_read_probe_rejects_a_leaked_control_project(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, 'sonar-credentials.json').write_text(json.dumps({'reader_token': 'token'}))
            Path(directory, 'sonar-admin-password').write_text('password')
            reader, admin = Mock(), Mock()
            reader.request.side_effect = [
                (200, sonar.VERSION), (200, {}), (200, {}), (404, {}),
                (200, {'components': [{'key': key} for key in (*sonar.PROJECTS, sonar.CONTROL)]})]
            with patch.object(sonar, 'API', side_effect=(reader, admin)):
                with self.assertRaisesRegex(RuntimeError, 'isolation'):
                    sonar.acceptance('https://source.test', directory)


if __name__ == '__main__':
    unittest.main()
