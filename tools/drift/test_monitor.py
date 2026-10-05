import copy
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import monitor as m


def snapshot(text="plan USD 5 per account-month, 10 million included", provider="cloudflare"):
    return {"kind": "pricing-document", "provider": provider, "url": "https://developers.cloudflare.com/workers/platform/pricing/",
            "records": {"document": {"text": text}}}


def finding(old="USD 5 per month", new="USD 6 per month"):
    initial = m.advance(None, {"price": snapshot(old)}, {}, "day1")
    return m.advance(initial, {"price": snapshot(new)}, {}, "day2")["findings"]["price"]


def issue(f, state="open", number=1, notes=""):
    title, body = m.issue_text(f)
    return {"number": number, "state": state, "user": {"login": m.BOT_LOGIN}, "title": title, "body": body + notes}


class MonitorTests(unittest.TestCase):
    def test_first_baseline_has_no_drift_flood(self):
        state = m.advance(None, {str(i): snapshot() for i in range(1000)}, {}, "day1")
        self.assertEqual(state["findings"], {})
        self.assertEqual(len(state["sources"]), 1000)

    def test_unchanged_does_not_create_events_but_updates_health_date(self):
        state = m.advance(None, {"p": snapshot()}, {}, "day1")
        state = m.advance(state, {"p": snapshot()}, {}, "day2")
        self.assertEqual(state["findings"], {})
        self.assertEqual(state["sources"]["p"]["lastSuccessfulCheck"], "day2")

    def test_failures_preserve_last_good_and_group_health(self):
        old = m.advance(None, {"a": snapshot(), "b": snapshot()}, {}, "day1")
        errors = {"a": {"error": "HTTP 503"}, "b": {"error": "missing credentials"}}
        state = m.advance(old, {}, errors, "day2")
        self.assertEqual(state["sources"], old["sources"])
        self.assertEqual(list(state["findings"]), ["monitor:source-health"])
        recovered = m.advance(state, {"a": snapshot(), "b": snapshot()}, {}, "day3")
        self.assertTrue(recovered["findings"]["monitor:source-health"]["recovered"])

    def test_invalid_snapshot_never_becomes_baseline(self):
        for bad in ({}, {"records": {}}, {"kind": "x", "records": {"x": 1}}):
            with self.assertRaises(ValueError):
                m.advance(None, {"bad": bad}, {}, "day1")

    def test_corrupt_state_does_not_silently_reset(self):
        with self.assertRaises(ValueError):
            m.advance({"schema": 999, "sources": {}}, {}, {}, "day1")

    def test_currency_unit_tier_term_and_inclusions_are_changes(self):
        for old, new in [
            ("5 USD", "5 EUR"), ("5 per GB", "5 per GiB"),
            ("first 10 GB", "first 100 GB"), ("12 month renewal", "12 month initial campaign"),
            ("10 million included", "5 million included"), ("tax included", "tax excluded"),
            ("5 per region", "5 per account"), ("valid until October", "valid until November"),
        ]:
            with self.subTest(new=new):
                self.assertEqual(finding(old, new)["delta"]["changed"], ["document"])

    def test_same_amount_moved_to_different_plan_is_a_change(self):
        f = finding("Small 500円; Large 1000円", "Small 1000円; Large 500円")
        self.assertEqual(f["delta"]["changed"], ["document"])

    def test_dedup_identical_and_closed_evidence(self):
        f = finding()
        self.assertEqual(m.issue_actions({"p": f}, [issue(f)]), [])
        self.assertEqual(m.issue_actions({"p": f}, [issue(f, "closed")]), [])

    def test_new_evidence_after_closed_creates_new_not_reopen(self):
        prior, current = finding(), finding(new="USD 7 per month")
        actions = m.issue_actions({"p": current}, [issue(prior, "closed")])
        self.assertEqual(actions[0]["action"], "create")
        self.assertIn("earlier review: #1", actions[0]["body"])

    def test_full_reversion_does_not_create_an_empty_issue(self):
        state = m.advance(None, {"price": snapshot("A")}, {}, "d1")
        state = m.advance(state, {"price": snapshot("B")}, {}, "d2")
        prior = copy.deepcopy(state["findings"]["price"])
        state = m.advance(state, {"price": snapshot("A")}, {}, "d3")
        self.assertTrue(state["findings"]["price"]["reverted"])
        self.assertEqual(m.issue_actions(state["findings"], []), [])
        actions = m.issue_actions(state["findings"], [issue(prior)])
        self.assertEqual(len(actions), 1)
        self.assertEqual(actions[0]["action"], "update")
        self.assertIn("returned to the original baseline", actions[0]["body"])
        self.assertEqual(m.issue_actions(state["findings"], [issue(prior, "closed")]), [])

    def test_ignored_subject_is_not_updated_or_recreated(self):
        old, new = finding(), finding(new="USD 9 per month")
        for status in ("open", "closed"):
            record = issue(old, status)
            record["labels"] = [{"name": "drift:ignore"}]
            self.assertEqual(m.issue_actions({"p": new}, [record]), [])
            record["labels"] = []
            self.assertEqual(len(m.issue_actions({"p": new}, [record])), 1)

    def test_partial_page_with_price_retains_last_good_snapshot(self):
        good = snapshot("Detailed table USD 5 per GB with units and terms\n" * 100)
        state = m.advance(None, {"p": good}, {}, "d1")
        bad = snapshot("Overview USD 0.00, full tables unavailable " * 5)
        result = m.advance(state, {"p": bad}, {}, "d2")
        self.assertEqual(result["sources"]["p"], state["sources"]["p"])
        self.assertEqual(result["health"]["failed"], 1)
        self.assertEqual(result["health"]["successful"], 0)
        self.assertNotIn("p", result["findings"])
        normal = snapshot(good["records"]["document"]["text"].replace("USD 5 per GB", "USD 6 per GiB"))
        updated = m.advance(state, {"p": normal}, {}, "d3")
        self.assertEqual(updated["health"]["failed"], 0)
        self.assertIn("p", updated["findings"])

    def test_update_preserves_human_notes_and_title(self):
        prior, current = finding(), finding(new="USD 7 per month")
        actions = m.issue_actions({"p": current}, [issue(prior, notes="\nMaintainer: investigate next week")])
        self.assertEqual(actions[0]["action"], "update")
        self.assertNotIn("title", actions[0])
        self.assertTrue(actions[0]["body"].endswith("Maintainer: investigate next week"))

    def test_retry_after_one_publication_is_idempotent(self):
        a, b = finding(), finding(new="USD 8 per month")
        b["subject"] = "second"
        actions = m.issue_actions({"a": a, "b": b}, [issue(a)])
        self.assertEqual(len(actions), 1)
        self.assertIn('"subject": "second"', actions[0]["body"])

    def test_removed_bot_block_stops_overwrite(self):
        old, new = finding(), finding(new="new")
        record = issue(old)
        record["body"] = record["body"].replace("<!-- archgopher-drift:start -->", "human edited")
        actions = m.issue_actions({"x": new}, [record])
        self.assertEqual(actions[0]["action"], "blocked")
        another = finding(new="USD 8"); another["subject"] = "another"
        actions = m.issue_actions({"x": new, "y": another}, [record])
        self.assertEqual([a["action"] for a in actions], ["blocked", "create"])

    def test_snapshot_version_only_is_not_schema_change(self):
        first = snapshot(); first["version"] = "1"
        second = copy.deepcopy(first); second["version"] = "2"
        state = m.advance(m.advance(None, {"p": first}, {}, "d1"), {"p": second}, {}, "d2")
        self.assertEqual(state["findings"], {})

    def test_html_ignores_navigation_but_keeps_plan_context(self):
        page = b'<html><nav>Random 44</nav><main><h1>Pricing</h1><p>Workers Paid plan monthly USD 5 per account. 10 million requests included, extra usage is charged per million requests.</p><table><tr><th>Term</th><th>Renewal</th></tr><tr><td>12 months</td><td>500 yen</td></tr></table></main></html>'
        result = m.pricing_snapshot('https://www.cloudflare.com/plans/', {"provider": "cloudflare"}, lambda _: page)
        text = result["records"]["document"]["text"]
        self.assertIn("12 months | 500 yen", text)
        self.assertNotIn("Random", text)

    def test_empty_captcha_and_partial_responses_are_errors(self):
        for payload in [b'', b'<html>Pricing</html>', b'<p>Just a moment verify you are human captcha pricing ' + b'x' * 200 + b'</p>']:
            with self.assertRaises(ValueError):
                m.pricing_snapshot('https://www.cloudflare.com/plans/', {"provider": "cloudflare"}, lambda _: payload)

    def test_no_price_snapshot_on_fetch_failure(self):
        def fail(_): raise RuntimeError("HTTP 503")
        snapshots, errors = m.collect_prices({'https://www.cloudflare.com/plans/': {"provider": "cloudflare"}}, fail)
        self.assertFalse(snapshots)
        self.assertEqual(len(errors), 1)

    def test_issue_identifies_exact_official_resource_definition_source(self):
        f = finding()
        f["snapshot"].update(kind="terraform-schema", provider="aws", version="6.0.0",
                             providerAddress="registry.terraform.io/hashicorp/aws",
                             schemaCommand="terraform providers schema -json", schemaCollection="resource_schemas")
        _, body = m.issue_text(f)
        self.assertIn("registry.terraform.io/hashicorp/aws@6.0.0", body)
        self.assertIn("terraform providers schema -json (resource_schemas)", body)

    def test_partial_http_and_length_mismatch_are_errors(self):
        class Response(io.BytesIO):
            def __init__(self, status, length):
                super().__init__(b"body")
                self.status = status
                self.headers = {"Content-Length": str(length)}
        for status, length in [(206, 4), (200, 40)]:
            http = m.HTTP()
            with patch.object(http.opener, "open", return_value=Response(status, length)):
                with self.assertRaises(ValueError):
                    http.request("https://www.cloudflare.com/plans/")

    def test_url_allowlist_and_credentials_boundary(self):
        for bad in ['http://aws.amazon.com/', 'https://evil.example/', 'https://api.github.com@evil.example/', 'https://api.github.com:444/']:
            with self.assertRaises(ValueError): m.checked_url(bad)
        self.assertEqual(m.checked_url('https://api.github.com/repos/a/b'), 'https://api.github.com/repos/a/b')

    def test_scope_excludes_fx_and_uses_only_catalog_regions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / 'catalog/conoha/server/books'; path.mkdir(parents=True)
            (path.parent / 'resource.yaml').write_text('type: conohavps_instance')
            (path / 'prices.json').write_text(json.dumps({
                'fx.jpy': {'source': 'https://example.invalid/fx'},
                'price': {'source': 'https://vps.conoha.jp/pricing/#linux', 'values': {'*': {'value': 1}}}}))
            providers, pages = m.catalog_scope(root)
            self.assertEqual(list(pages), ['https://vps.conoha.jp/pricing/'])
            self.assertEqual(pages['https://vps.conoha.jp/pricing/']['regions'], ['*'])

    def test_issue_inventory_must_be_complete(self):
        class Broken:
            def json(self, _): return {"message": "rate limited"}
        with self.assertRaises(ValueError): m.list_issues(Broken(), 'x/y')


if __name__ == '__main__':
    unittest.main()
