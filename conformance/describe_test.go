package conformance_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/NerdsWhoFish/dusk-plugin-sdk/conformance"
	duskv1alpha1 "github.com/NerdsWhoFish/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

func good() *duskv1alpha1.DescribeResponse {
	return &duskv1alpha1.DescribeResponse{
		PluginId:      "airtrail",
		Version:       "v1.2.3",
		SchemaVersion: conformance.SchemaVersion,
		EmitsKinds:    []string{"flight", "airport"},
		ConfigFields: []*duskv1alpha1.ConfigField{{
			Name:  "base_url",
			Label: "AirTrail URL",
			Type:  duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_STRING,
		}},
		Actions: []*duskv1alpha1.ActionDescriptor{{
			Name:           "delete_flight",
			Description:    "Remove a flight from the logbook.",
			Class:          duskv1alpha1.ActionClass_ACTION_CLASS_DESTRUCTIVE,
			ProofFrom:      "get",
			AppliesToKinds: []string{"flight"},
		}},
		Ui: []*duskv1alpha1.UIContribution{{
			Element: "airtrail-flights",
			Asset:   "airtrail-flights.js",
			Title:   "Flights",
		}},
		Budget: &duskv1alpha1.SourceBudget{KeyFields: []string{"base_url"}},
	}
}

func object(t *testing.T, value map[string]any) *structpb.Struct {
	t.Helper()

	built, err := structpb.NewStruct(value)
	if err != nil {
		t.Fatalf("build struct: %v", err)
	}
	return built
}

func TestValidateDescribe(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, *duskv1alpha1.DescribeResponse)
		wants   string
		accepts bool
	}{
		{name: "a complete description", mutate: func(*testing.T, *duskv1alpha1.DescribeResponse) {}, accepts: true},
		{
			name:   "a plugin id the marketplace could not name a repository from",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.PluginId = "Air Trail" },
			wants:  "plugin_id",
		},
		{
			name:   "a schema version from another contract",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.SchemaVersion = "v1beta1" },
			wants:  "schema_version",
		},
		{
			name:   "the same kind emitted twice",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.EmitsKinds = []string{"flight", "flight"} },
			wants:  "duplicate kind",
		},
		{
			name:   "an action with no class to draw approval defaults from",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.Actions[0].Class = 0 },
			wants:  "actions[0].class",
		},
		{
			name:   "an action nothing describes",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.Actions[0].Description = "" },
			wants:  "actions[0].description",
		},
		{
			name: "two actions of one name",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) {
				d.Actions = append(d.Actions, &duskv1alpha1.ActionDescriptor{
					Name:        "delete_flight",
					Description: "Again.",
					Class:       duskv1alpha1.ActionClass_ACTION_CLASS_READ_ONLY,
				})
			},
			wants: "duplicate action",
		},
		{
			name: "a params schema that is not an object",
			mutate: func(t *testing.T, d *duskv1alpha1.DescribeResponse) {
				d.Actions[0].ParamsSchema = object(t, map[string]any{"type": "string"})
			},
			wants: "params_schema.type",
		},
		{
			name:   "an element with no hyphen, which is not a custom element",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.Ui[0].Element = "flights" },
			wants:  "ui[0].element",
		},
		{
			name:   "an element with no JavaScript defining it",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.Ui[0].Asset = "" },
			wants:  "ui[0].asset",
		},
		{
			name: "a contribution that is both declared and drawn",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) {
				d.Ui[0].Spec = &duskv1alpha1.ViewSpec{
					Layout: duskv1alpha1.ViewLayout_VIEW_LAYOUT_TABLE,
					Fields: []*duskv1alpha1.ViewField{{Source: "ref"}},
				}
			},
			wants: "never both",
		},
		{
			name: "a declared view of no fields",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) {
				d.Ui[0].Element, d.Ui[0].Asset = "", ""
				d.Ui[0].Spec = &duskv1alpha1.ViewSpec{Layout: duskv1alpha1.ViewLayout_VIEW_LAYOUT_TABLE}
			},
			wants: "spec.fields",
		},
		{
			name: "a plugin-page view limited by entity kind",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) {
				d.Ui[0].Slot = duskv1alpha1.UISlot_UI_SLOT_PLUGIN
				d.Ui[0].AppliesToKinds = []string{"flight"}
			},
			wants: "applies_to_kinds",
		},
		{
			name:   "a budget keyed on a field nothing declares",
			mutate: func(_ *testing.T, d *duskv1alpha1.DescribeResponse) { d.Budget.KeyFields = []string{"endpoint"} },
			wants:  "not a declared config field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			described := good()
			test.mutate(t, described)

			result := conformance.ValidateDescribe(described)
			if test.accepts {
				if !result.OK() {
					t.Fatalf("expected a clean description, got:\n%s", result.Error())
				}
				return
			}

			if result.OK() {
				t.Fatalf("expected a violation mentioning %q, got none", test.wants)
			}
			if !strings.Contains(result.Error(), test.wants) {
				t.Fatalf("expected a violation mentioning %q, got:\n%s", test.wants, result.Error())
			}
		})
	}
}

// ADR-0015 requires a mutating action to name the read that yields a proof
// token, because an agent meeting the read-before-write contract with nothing
// naming the call that satisfies it has no way forward.
func TestADR0015_MutatingActionNamesItsProofRead(t *testing.T) {
	classes := map[duskv1alpha1.ActionClass]bool{
		duskv1alpha1.ActionClass_ACTION_CLASS_READ_ONLY:   true,
		duskv1alpha1.ActionClass_ACTION_CLASS_MUTATING:    false,
		duskv1alpha1.ActionClass_ACTION_CLASS_DESTRUCTIVE: false,
	}

	for class, allowed := range classes {
		t.Run(class.String(), func(t *testing.T) {
			described := good()
			described.Actions[0].Class = class
			described.Actions[0].ProofFrom = ""

			result := conformance.ValidateDescribe(described)
			if result.OK() != allowed {
				t.Fatalf("%s with no proof_from: accepted=%v, wanted %v\n%s",
					class, result.OK(), allowed, result.Error())
			}
		})
	}
}

// ADR-0015 lets an action declare what may run after it so approval can cover
// the chain. A declaration states what may follow, never with what: params
// belong to the invocation that returns the step.
func TestADR0015_DeclaredCompositionCarriesNoParams(t *testing.T) {
	described := good()
	described.Actions[0].Then = []*duskv1alpha1.Next{{
		Action: "restart",
		Params: object(t, map[string]any{"force": true}),
	}}

	result := conformance.ValidateDescribe(described)
	if result.OK() {
		t.Fatal("expected a declared composition step carrying params to be refused")
	}
	if !strings.Contains(result.Error(), "then[0].params") {
		t.Fatalf("expected the violation to name the step's params, got:\n%s", result.Error())
	}
}
