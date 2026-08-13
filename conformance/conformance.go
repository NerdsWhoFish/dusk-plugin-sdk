// Package conformance validates a plugin's output against the Dusk contract.
//
// Plugin authors run this against their own output so "is my plugin correct"
// has an answer that does not involve reading the Dusk server.
package conformance

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	duskv1alpha1 "github.com/NerdsWhoFish/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

// SchemaVersion is the only version this package accepts.
const SchemaVersion = "v1alpha1"

// Violation is one contract breach, located well enough to fix.
type Violation struct {
	Path    string
	Message string
}

func (v Violation) String() string { return v.Path + ": " + v.Message }

// Result is what a plugin author reads.
type Result struct {
	Batch      *duskv1alpha1.IngestBatch
	Violations []Violation
}

// OK reports whether the batch is contract-clean.
func (r Result) OK() bool { return len(r.Violations) == 0 }

func (r Result) Error() string {
	if r.OK() {
		return ""
	}
	parts := make([]string, 0, len(r.Violations))
	for _, v := range r.Violations {
		parts = append(parts, v.String())
	}
	return strings.Join(parts, "\n")
}

// ValidateBatch parses protojson emitted by a Tier 1 ingester and checks it
// against the contract. Malformed JSON yields a Result with a single violation
// rather than a separate error path, so callers have one thing to inspect.
func ValidateBatch(data []byte) Result {
	batch := &duskv1alpha1.IngestBatch{}
	if err := protojson.Unmarshal(data, batch); err != nil {
		return Result{Violations: []Violation{{Path: "$", Message: fmt.Sprintf("invalid protojson: %v", err)}}}
	}

	r := Result{Batch: batch}

	if batch.GetSchemaVersion() != SchemaVersion {
		r.Violations = append(r.Violations, Violation{
			Path:    "schema_version",
			Message: fmt.Sprintf("must be %q, got %q", SchemaVersion, batch.GetSchemaVersion()),
		})
	}

	for i, e := range batch.GetEntities() {
		r.Violations = append(r.Violations, validateEntity(fmt.Sprintf("entities[%d]", i), e)...)
	}
	for i, rel := range batch.GetRelations() {
		r.Violations = append(r.Violations, validateRelation(fmt.Sprintf("relations[%d]", i), rel)...)
	}
	for i, n := range batch.GetNotes() {
		r.Violations = append(r.Violations, validateNote(fmt.Sprintf("notes[%d]", i), n)...)
	}
	for i, o := range batch.GetObservations() {
		r.Violations = append(r.Violations, validateObservation(fmt.Sprintf("observations[%d]", i), o)...)
	}

	return r
}

// CanonicalRef builds the stable identity form "kind:namespace/name".
func CanonicalRef(kind, namespace, name string) string {
	return kind + ":" + namespace + "/" + name
}

// ParseRef splits a canonical ref. It rejects anything it cannot round-trip,
// because a ref that does not round-trip breaks correlation between sources.
func ParseRef(ref string) (kind, namespace, name string, err error) {
	colon := strings.Index(ref, ":")
	if colon <= 0 {
		return "", "", "", fmt.Errorf("ref %q: missing kind prefix", ref)
	}
	slash := strings.Index(ref[colon+1:], "/")
	if slash < 0 {
		return "", "", "", fmt.Errorf("ref %q: missing namespace separator", ref)
	}

	kind = ref[:colon]
	namespace = ref[colon+1 : colon+1+slash]
	name = ref[colon+2+slash:]

	if name == "" {
		return "", "", "", fmt.Errorf("ref %q: empty name", ref)
	}
	if strings.Contains(name, "/") {
		return "", "", "", fmt.Errorf("ref %q: name contains %q", ref, "/")
	}
	return kind, namespace, name, nil
}

func validateEntity(path string, e *duskv1alpha1.Entity) []Violation {
	var v []Violation

	switch {
	case e.GetRef() == "":
		v = append(v, Violation{path + ".ref", "required"})
	default:
		kind, ns, name, err := ParseRef(e.GetRef())
		switch {
		case err != nil:
			v = append(v, Violation{path + ".ref", err.Error()})
		case kind != e.GetKind() || ns != e.GetNamespace() || name != e.GetName():
			v = append(v, Violation{
				path + ".ref",
				fmt.Sprintf("must equal %q", CanonicalRef(e.GetKind(), e.GetNamespace(), e.GetName())),
			})
		}
	}

	if e.GetKind() == "" {
		v = append(v, Violation{path + ".kind", "required"})
	}
	if e.GetName() == "" {
		v = append(v, Violation{path + ".name", "required"})
	}
	return append(v, validateProvenance(path, e.GetProvenance())...)
}

func validateRelation(path string, r *duskv1alpha1.Relation) []Violation {
	var v []Violation
	for field, val := range map[string]string{"from": r.GetFrom(), "to": r.GetTo(), "type": r.GetType()} {
		if val == "" {
			v = append(v, Violation{path + "." + field, "required"})
		}
	}
	return append(v, validateProvenance(path, r.GetProvenance())...)
}

func validateNote(path string, n *duskv1alpha1.Note) []Violation {
	var v []Violation
	if len(n.GetRefs()) == 0 {
		v = append(v, Violation{path + ".refs", "at least one ref required"})
	}
	if n.GetBody() == "" {
		v = append(v, Violation{path + ".body", "required"})
	}
	if n.GetKind() == "" {
		v = append(v, Violation{path + ".kind", "required"})
	}
	return append(v, validateProvenance(path, n.GetProvenance())...)
}

func validateObservation(path string, o *duskv1alpha1.Observation) []Violation {
	var v []Violation
	if o.GetRef() == "" {
		v = append(v, Violation{path + ".ref", "required"})
	}
	return append(v, validateProvenance(path, o.GetProvenance())...)
}

// Provenance is required everywhere: Dusk never infers where a claim came from.
func validateProvenance(path string, p *duskv1alpha1.Provenance) []Violation {
	if p.GetSource() == "" {
		return []Violation{{path + ".provenance.source", "required"}}
	}
	return nil
}
