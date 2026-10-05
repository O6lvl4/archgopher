"""Exercise the actual baseline-selection script with offline GitHub responses."""
import json
from pathlib import Path
import shutil
import subprocess
import textwrap
import unittest


@unittest.skipUnless(shutil.which("node"), "Node is needed to test the github-script step")
class BaselineWorkflowTests(unittest.TestCase):
    def select(self, runs, artifacts=None, steps=None):
        workflow = (Path(__file__).resolve().parents[2] / '.github/workflows/drift.yml').read_text()
        script = textwrap.dedent(workflow.split('          script: |\n', 1)[1].split('      - name:', 1)[0])
        inputs = json.dumps({'runs': runs, 'artifacts': artifacts or {}, 'steps': steps or {}})
        harness = '''
const input = INPUT;
const result = {};
const context = {repo: {owner: 'O6lvl4', repo: 'archgopher'}, runId: 100,
  payload: {repository: {default_branch: 'main'}}};
const core = {setOutput: (key, value) => result[key] = value, setFailed: value => result.error = value};
const actions = {listWorkflowRuns: 'runs', listWorkflowRunArtifacts: 'artifacts', listJobsForWorkflowRun: 'jobs'};
const github = {rest: {actions}, paginate: async (method, args) => {
  if (method === 'runs') return input.runs;
  if (method === 'artifacts') return input.artifacts[args.run_id] || [];
  if (method === 'jobs') return [{steps: input.steps[args.run_id] || []}];
  throw Error('unexpected endpoint');
}};
(async () => { SCRIPT; console.log(JSON.stringify(result)); })().catch(e => {console.error(e); process.exit(1)});
'''.replace('INPUT', inputs).replace('SCRIPT', '(await (async () => {' + script + '})())')
        return json.loads(subprocess.run(['node', '-e', harness], check=True, capture_output=True, text=True).stdout)

    def run_record(self, **kwargs):
        return {'id': 1, 'event': 'schedule', 'head_repository': {'full_name': 'O6lvl4/archgopher'},
                'head_branch': 'main', 'conclusion': 'failure', **kwargs}

    def test_first_run_and_failed_setup_allow_bootstrap(self):
        self.assertEqual(self.select([]), {})
        self.assertEqual(self.select([self.run_record()], steps={'1': [
            {'name': 'Detect changes', 'status': 'completed', 'conclusion': 'skipped'}]}), {})

    def test_attempted_detection_or_expired_state_cannot_silently_reset(self):
        run = self.run_record()
        self.assertIn('error', self.select([run], steps={'1': [
            {'name': 'Detect changes', 'status': 'completed', 'conclusion': 'failure'}]}))
        self.assertIn('error', self.select([run], artifacts={'1': [{'name': 'catalog-drift-state', 'expired': True}]}))
        self.assertIn('error', self.select([self.run_record(conclusion='success')]))

    def test_restore_retained_trusted_state_and_reject_untrusted_runs(self):
        state = [{'name': 'catalog-drift-state', 'expired': False}]
        self.assertEqual(self.select([self.run_record()], artifacts={'1': state}), {'run': '1'})
        for change in [{'event': 'pull_request'}, {'head_branch': 'feature'},
                       {'head_repository': {'full_name': 'someone/fork'}}, {'id': 100}]:
            self.assertEqual(self.select([self.run_record(**change)], artifacts={'1': state}), {})


if __name__ == '__main__':
    unittest.main()
