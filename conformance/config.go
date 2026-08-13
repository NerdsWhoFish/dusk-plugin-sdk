package conformance

import (
	"fmt"
	"regexp"

	duskv1alpha1 "github.com/NerdsWhoFish/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

var fieldNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidateConfigFields checks a plugin's declared configuration form.
func ValidateConfigFields(fields []*duskv1alpha1.ConfigField) Result {
	var r Result
	seen := make(map[string]bool, len(fields))

	for i, f := range fields {
		path := fmt.Sprintf("config_fields[%d]", i)
		r.Violations = append(r.Violations, validateFieldName(path, f.GetName(), seen)...)
		r.Violations = append(r.Violations, validateFieldShape(path, f)...)
	}

	return r
}

func validateFieldName(path, name string, seen map[string]bool) []Violation {
	switch {
	case name == "":
		return []Violation{{path + ".name", "required"}}
	case !fieldNamePattern.MatchString(name):
		return []Violation{{path + ".name", "must match " + fieldNamePattern.String()}}
	case seen[name]:
		return []Violation{{path + ".name", fmt.Sprintf("duplicate field %q", name)}}
	}
	seen[name] = true
	return nil
}

func validateFieldShape(path string, f *duskv1alpha1.ConfigField) []Violation {
	var v []Violation

	if f.GetLabel() == "" {
		v = append(v, Violation{path + ".label", "required, it is the form label"})
	}
	if f.GetType() == duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_UNSPECIFIED {
		v = append(v, Violation{path + ".type", "required, Dusk cannot render an unspecified type"})
	}
	if f.GetType() == duskv1alpha1.ConfigFieldType_CONFIG_FIELD_TYPE_ENUM && len(f.GetEnumValues()) == 0 {
		v = append(v, Violation{path + ".enum_values", "required when type is ENUM"})
	}
	if p := f.GetPattern(); p != "" {
		if _, err := regexp.Compile(p); err != nil {
			v = append(v, Violation{path + ".pattern", "does not compile: " + err.Error()})
		}
	}

	// A shipped default for a secret is a credential in the plugin binary.
	if f.GetSensitive() && f.GetDefaultValue() != "" {
		v = append(v, Violation{
			path + ".default_value",
			"a sensitive field must not carry a default, that ships a credential",
		})
	}

	return v
}
