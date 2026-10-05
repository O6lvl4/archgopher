import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import providers
import smoke
from test_providers import FakeTerraform, document


class SmokeTests(unittest.TestCase):
    def test_all_smoke_pins_use_production_export_and_readonly_checksums(self):
        for provider, (source, prefix) in providers.REGISTRY.items():
            with self.subTest(provider=provider), tempfile.TemporaryDirectory() as directory:
                name = prefix + 'example'
                fake = FakeTerraform(document(source, (name,)))
                calls = []

                def run(command, **kwargs):
                    lock = (Path(kwargs['cwd']) / '.terraform.lock.hcl').read_text()
                    self.assertIn('h1:', lock)
                    self.assertIn('zh:', lock)
                    self.assertIn(f'provider "registry.terraform.io/{source}"', lock)
                    self.assertNotIn('GH_TOKEN', kwargs['env'])
                    calls.append(command)
                    return fake(command, **kwargs)

                out = Path(directory) / 'report.json'
                with contextlib.redirect_stdout(io.StringIO()), patch.object(smoke.monitor, 'catalog_scope', return_value=({provider: {'resources': [name], 'free': []}}, {})):
                    self.assertEqual(smoke.verify(provider, directory, out, run=run), 0)
                self.assertIn('-lockfile=readonly', calls[0])
                self.assertEqual(calls[1], ['terraform', 'providers', 'schema', '-json'])
                report = json.loads(out.read_text())
                self.assertTrue(report['success'])
                self.assertEqual(report['resourceCount'], 1)
                self.assertEqual(report['coverage']['present'], 1)

    def test_export_failure_is_recorded_and_fails_the_smoke_test(self):
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory) / 'report.json'
            with contextlib.redirect_stdout(io.StringIO()), patch.object(smoke.monitor, 'catalog_scope', return_value=({'aws': {'resources': [], 'free': []}}, {})):
                self.assertEqual(smoke.verify('aws', directory, out, run=FakeTerraform(fail='providers')), 1)
            report = json.loads(out.read_text())
            self.assertFalse(report['success'])
            self.assertNotIn('resourceCount', report)
            self.assertIn('provider:aws', report['errors'])

    def test_source_pin_cannot_substitute_another_publisher(self):
        with patch.object(smoke.monitor, 'read_json', return_value={'aws': {'source': 'other/aws', 'version': '1.0.0'}}):
            with self.assertRaises(ValueError):
                smoke.verify('aws', '.', '/unused')


if __name__ == '__main__':
    unittest.main()
