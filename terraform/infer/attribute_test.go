package infer

import (
	"testing"

	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/terraform/eval"
)

func TestReadAttributeCountsAcrossBlocks(t *testing.T) {
	r := &eval.Resource{
		Attrs: map[string]any{
			"email_receiver": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}},
			"criteria": []any{
				map[string]any{"dimension": []any{map[string]any{"values": []any{"x", "y"}}}},
				map[string]any{"dimension": []any{map[string]any{"values": []any{"z"}}, map[string]any{"values": []any{"*"}}}},
				map[string]any{"metric_name": "cpu"},
			},
			"scopes": []any{},
		},
		Refs: map[string][]string{"scopes": {"azurerm_linux_web_app.a", "azurerm_linux_web_app.b"}},
	}
	for path, want := range map[string]any{
		"email_receiver.#":            2.0,
		"criteria.#":                  3.0,
		"criteria.dimension.#":        3.0,
		"criteria.dimension.values.#": 4.0,
		// Nothing written leaves the field to its default.
		"sms_receiver.#": nil,
		// Ids known only after apply are counted by what the list references.
		"scopes.#": 2.0,
	} {
		got := readAttribute(r, field.Field{Key: "k", Type: field.Number, Path: path})
		if got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}
}

func TestReadAttributeFlagReadsWhetherWritten(t *testing.T) {
	r := &eval.Resource{
		Attrs: map[string]any{
			"enabled":             false,
			"partner_solution_id": "/subscriptions/x/partner",
			"storage_account_id":  "",
			"aad_auth":            []any{map[string]any{}},
		},
		Refs: map[string][]string{"log_analytics_workspace_id": {"azurerm_log_analytics_workspace.logs"}},
	}
	for path, want := range map[string]any{
		"enabled":             false,
		"partner_solution_id": true,
		"aad_auth":            true,
		// An empty string is not written.
		"storage_account_id": nil,
		// An id known only after apply is still written.
		"log_analytics_workspace_id":     true,
		"eventhub_authorization_rule_id": nil,
	} {
		if got := readAttribute(r, field.Field{Key: "k", Type: field.Flag, Path: path}); got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}
}
