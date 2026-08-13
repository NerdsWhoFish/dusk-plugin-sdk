package conformance

import (
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	duskv1alpha1 "github.com/NerdsWhoFish/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

var (
	pluginIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	actionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// ValidateDescribe checks what a plugin says about itself. The form, the
// actions, the views and the budget are built from this one answer, so a
// wrong description fails later, in somebody else's component.
func ValidateDescribe(described *duskv1alpha1.DescribeResponse) Result {
	var r Result

	if described == nil {
		return Result{Violations: []Violation{{"$", "no description at all"}}}
	}

	switch id := described.GetPluginId(); {
	case id == "":
		r.Violations = append(r.Violations, Violation{"plugin_id", "required"})
	case !pluginIDPattern.MatchString(id):
		r.Violations = append(r.Violations, Violation{"plugin_id", "must match " + pluginIDPattern.String() + ", because it names the repository the marketplace finds"})
	}

	if described.GetVersion() == "" {
		r.Violations = append(r.Violations, Violation{"version", "required, it is what an update is judged against"})
	}
	if v := described.GetSchemaVersion(); v != SchemaVersion {
		r.Violations = append(r.Violations, Violation{"schema_version", fmt.Sprintf("must be %q, got %q", SchemaVersion, v)})
	}

	kinds := map[string]bool{}
	for i, kind := range described.GetEmitsKinds() {
		path := fmt.Sprintf("emits_kinds[%d]", i)
		switch {
		case kind == "":
			r.Violations = append(r.Violations, Violation{path, "required"})
		case kinds[kind]:
			r.Violations = append(r.Violations, Violation{path, fmt.Sprintf("duplicate kind %q", kind)})
		}
		kinds[kind] = true
	}

	fields := ValidateConfigFields(described.GetConfigFields())
	r.Violations = append(r.Violations, fields.Violations...)

	declared := map[string]bool{}
	for _, field := range described.GetConfigFields() {
		declared[field.GetName()] = true
	}

	names := map[string]bool{}
	for i, action := range described.GetActions() {
		r.Violations = append(r.Violations, validateAction(fmt.Sprintf("actions[%d]", i), action, names)...)
	}
	for i, ui := range described.GetUi() {
		r.Violations = append(r.Violations, validateUI(fmt.Sprintf("ui[%d]", i), ui)...)
	}
	r.Violations = append(r.Violations, validateBudget(described.GetBudget(), declared)...)

	return r
}

func validateAction(path string, action *duskv1alpha1.ActionDescriptor, seen map[string]bool) []Violation {
	var v []Violation

	switch name := action.GetName(); {
	case name == "":
		v = append(v, Violation{path + ".name", "required"})
	case !actionNamePattern.MatchString(name):
		v = append(v, Violation{path + ".name", "must match " + actionNamePattern.String()})
	case seen[name]:
		v = append(v, Violation{path + ".name", fmt.Sprintf("duplicate action %q", name)})
	default:
		seen[name] = true
	}

	// An agent chooses an action by reading this. Without it the choice is made
	// from the name alone, which is how the wrong thing gets run.
	if action.GetDescription() == "" {
		v = append(v, Violation{path + ".description", "required, it is what an agent decides from"})
	}

	if action.GetClass() == duskv1alpha1.ActionClass_ACTION_CLASS_UNSPECIFIED {
		v = append(v, Violation{path + ".class", "required, it is what approval defaults are drawn from"})
	}

	// Read-before-write is a contract the agent has to be told how to satisfy,
	// and a rejection that cannot name the call that fixes it is a dead end.
	if mutates(action.GetClass()) && action.GetProofFrom() == "" {
		v = append(v, Violation{
			path + ".proof_from",
			"required for a mutating or destructive action, it names the read that yields a satisfying proof token",
		})
	}

	v = append(v, validateParamsSchema(path+".params_schema", action.GetParamsSchema())...)

	for i, next := range action.GetThen() {
		step := fmt.Sprintf("%s.then[%d]", path, i)
		if next.GetAction() == "" {
			v = append(v, Violation{step + ".action", "required, a composition step names the action it runs"})
		}
		// A declared step says what may follow, not with what. Params are known
		// only once the action producing them has run.
		if next.GetParams() != nil {
			v = append(v, Violation{step + ".params", "must be empty in a declaration, it is set by the invocation that returns the step"})
		}
	}

	return v
}

func mutates(class duskv1alpha1.ActionClass) bool {
	return class == duskv1alpha1.ActionClass_ACTION_CLASS_MUTATING ||
		class == duskv1alpha1.ActionClass_ACTION_CLASS_DESTRUCTIVE
}

// validateParamsSchema checks the shape Dusk and every agent assume. A bare
// string or array schema has no form to render and no named arguments to fill.
func validateParamsSchema(path string, schema *structpb.Struct) []Violation {
	if schema == nil {
		return nil
	}

	value := schema.AsMap()
	if len(value) == 0 {
		return nil
	}

	declared, ok := value["type"]
	if !ok {
		return []Violation{{path + ".type", `required, and must be "object"`}}
	}
	if declared != "object" {
		return []Violation{{path + ".type", fmt.Sprintf(`must be "object", got %v`, declared)}}
	}
	return nil
}

func validateUI(path string, ui *duskv1alpha1.UIContribution) []Violation {
	var v []Violation

	drawn := ui.GetElement() != "" || ui.GetAsset() != ""
	declaredView := ui.GetSpec() != nil

	switch {
	case drawn && declaredView:
		v = append(v, Violation{path, "set spec or set element and asset, never both: two renderings of one contribution is one too many"})
	case !drawn && !declaredView:
		v = append(v, Violation{path, "set spec for a declared view, or element and asset for one the plugin draws"})
	}

	if drawn {
		v = append(v, validateElement(path, ui)...)
	}

	if ui.GetTitle() == "" {
		v = append(v, Violation{path + ".title", "required, it labels the block the view is rendered in"})
	}

	// A plugin's own page is not about any one entity, so a kind list there
	// filters against something that will never be present.
	if ui.GetSlot() == duskv1alpha1.UISlot_UI_SLOT_PLUGIN && len(ui.GetAppliesToKinds()) > 0 {
		v = append(v, Violation{path + ".applies_to_kinds", "the plugin slot mounts no entity, so it cannot be limited by kind"})
	}

	if declaredView {
		v = append(v, validateViewSpec(path+".spec", ui.GetSpec())...)
	}
	return v
}

func validateElement(path string, ui *duskv1alpha1.UIContribution) []Violation {
	var v []Violation

	switch element := ui.GetElement(); {
	case element == "":
		v = append(v, Violation{path + ".element", "required when an asset is served"})
	case !strings.Contains(element, "-"):
		v = append(v, Violation{path + ".element", "must contain a hyphen, custom element names do"})
	case element != strings.ToLower(element):
		v = append(v, Violation{path + ".element", "must be lower case"})
	}

	if ui.GetAsset() == "" {
		v = append(v, Violation{path + ".asset", "required, it is the JavaScript defining the element"})
	}
	return v
}

func validateViewSpec(path string, spec *duskv1alpha1.ViewSpec) []Violation {
	var v []Violation

	if spec.GetLayout() == duskv1alpha1.ViewLayout_VIEW_LAYOUT_UNSPECIFIED {
		v = append(v, Violation{path + ".layout", "required, Dusk cannot render an unspecified layout"})
	}
	if len(spec.GetFields()) == 0 {
		v = append(v, Violation{path + ".fields", "at least one field required, a view of nothing shows nothing"})
	}

	for i, field := range spec.GetFields() {
		if field.GetSource() == "" {
			v = append(v, Violation{fmt.Sprintf("%s.fields[%d].source", path, i), "required, it names the entity field or attribute to read"})
		}
	}
	return v
}

func validateBudget(budget *duskv1alpha1.SourceBudget, declared map[string]bool) []Violation {
	if budget == nil {
		return nil
	}

	var v []Violation
	for i, field := range budget.GetKeyFields() {
		path := fmt.Sprintf("budget.key_fields[%d]", i)
		switch {
		case field == "":
			v = append(v, Violation{path, "required"})
		case !declared[field]:
			v = append(v, Violation{path, fmt.Sprintf("names %q, which is not a declared config field", field)})
		}
	}

	if budget.GetMaxConcurrent() < 0 {
		v = append(v, Violation{"budget.max_concurrent", "cannot be negative"})
	}
	if budget.GetMinSpacingSeconds() < 0 {
		v = append(v, Violation{"budget.min_spacing_seconds", "cannot be negative"})
	}
	return v
}
