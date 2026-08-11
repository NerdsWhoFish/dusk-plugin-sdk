package conformance_test

import (
	"testing"

	"github.com/FetchHQ/dusk-plugin-sdk/conformance"
	duskv1alpha1 "github.com/FetchHQ/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

func field(mut ...func(*duskv1alpha1.ConfigField)) *duskv1alpha1.ConfigField {
	f := &duskv1alpha1.ConfigField{
		Name:  "api_token",
		Label: "API token",
		Type:  duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_STRING,
	}
	for _, m := range mut {
		m(f)
	}
	return f
}

func TestValidateConfigFields(t *testing.T) {
	tests := []struct {
		name   string
		fields []*duskv1alpha1.ConfigField
		want   []string
	}{
		{
			name:   "a well-formed field is accepted",
			fields: []*duskv1alpha1.ConfigField{field()},
		},
		{
			name:   "no fields at all is valid, plugins may need no config",
			fields: nil,
		},
		{
			name:   "a field without a name is rejected",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) { f.Name = "" })},
			want:   []string{"config_fields[0].name"},
		},
		{
			name:   "a name that is not a lower snake identifier is rejected",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) { f.Name = "API-Token" })},
			want:   []string{"config_fields[0].name"},
		},
		{
			name:   "a duplicate name is rejected",
			fields: []*duskv1alpha1.ConfigField{field(), field()},
			want:   []string{"config_fields[1].name"},
		},
		{
			name:   "a field without a label is rejected, the form needs one",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) { f.Label = "" })},
			want:   []string{"config_fields[0].label"},
		},
		{
			name: "an unspecified type is rejected, it cannot be rendered",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) {
				f.Type = duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_UNSPECIFIED
			})},
			want: []string{"config_fields[0].type"},
		},
		{
			name: "an enum with no values is rejected",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) {
				f.Type = duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_ENUM
			})},
			want: []string{"config_fields[0].enum_values"},
		},
		{
			name: "an enum with values is accepted",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) {
				f.Type = duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_ENUM
				f.EnumValues = []string{"a", "b"}
			})},
		},
		{
			name:   "a pattern that does not compile is rejected",
			fields: []*duskv1alpha1.ConfigField{field(func(f *duskv1alpha1.ConfigField) { f.Pattern = "[unclosed" })},
			want:   []string{"config_fields[0].pattern"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conformance.ValidateConfigFields(tt.fields)
			assertViolations(t, got, tt.want)
		})
	}
}

// A sensitive field carrying a default ships a credential inside the plugin
// binary, which no amount of storage encryption can undo.
func TestADR0023_SensitiveFieldMayNotCarryADefault(t *testing.T) {
	tests := []struct {
		name         string
		sensitive    bool
		defaultValue string
		wantRejected bool
	}{
		{name: "sensitive with a default is rejected", sensitive: true, defaultValue: "sk-live-abc123", wantRejected: true},
		{name: "sensitive without a default is fine", sensitive: true},
		{name: "a default on a non-sensitive field is fine", defaultValue: "https://api.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := field(func(f *duskv1alpha1.ConfigField) {
				f.Sensitive = tt.sensitive
				f.DefaultValue = tt.defaultValue
			})
			got := conformance.ValidateConfigFields([]*duskv1alpha1.ConfigField{f})

			var want []string
			if tt.wantRejected {
				want = []string{"config_fields[0].default_value"}
			}
			assertViolations(t, got, want)
		})
	}
}

func assertViolations(t *testing.T, got conformance.Result, want []string) {
	t.Helper()

	if len(want) == 0 {
		if !got.OK() {
			t.Fatalf("want clean, got violations:\n%s", got.Error())
		}
		return
	}
	for _, path := range want {
		if !hasViolation(t, got, path) {
			t.Errorf("want violation at %q, got:\n%s", path, got.Error())
		}
	}
}
