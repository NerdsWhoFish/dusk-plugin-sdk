package conformance_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/FetchHQ/dusk-plugin-sdk/conformance"
	duskv1alpha1 "github.com/FetchHQ/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

// validEntityBatch is the shape a correct Tier 1 ingester emits.
const validEntityBatch = `{
  "schemaVersion": "v1alpha1",
  "entities": [{
    "ref": "service:home/scrypted",
    "kind": "service",
    "namespace": "home",
    "name": "scrypted",
    "provenance": {"source": "kubectl-sh", "observedAt": "2026-08-11T15:04:05Z"}
  }]
}`

func TestValidateBatch(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string // violation paths, empty means clean
	}{
		{
			name: "a correct batch is accepted",
			in:   validEntityBatch,
		},
		{
			name: "malformed json is reported rather than panicking",
			in:   `{"schemaVersion":`,
			want: []string{"$"},
		},
		{
			name: "a batch from a future schema version is rejected",
			in:   `{"schemaVersion": "v2", "entities": []}`,
			want: []string{"schema_version"},
		},
		{
			name: "a batch with no schema version is rejected",
			in:   `{"entities": []}`,
			want: []string{"schema_version"},
		},
		{
			name: "an entity whose ref disagrees with its fields is rejected",
			in: `{"schemaVersion":"v1alpha1","entities":[{
				"ref":"service:home/scrypted","kind":"service","namespace":"other","name":"scrypted",
				"provenance":{"source":"x"}}]}`,
			want: []string{"entities[0].ref"},
		},
		{
			name: "an entity with an unparseable ref is rejected",
			in: `{"schemaVersion":"v1alpha1","entities":[{
				"ref":"scrypted","kind":"service","name":"scrypted",
				"provenance":{"source":"x"}}]}`,
			want: []string{"entities[0].ref"},
		},
		{
			name: "an entity without provenance is rejected",
			in: `{"schemaVersion":"v1alpha1","entities":[{
				"ref":"service:home/scrypted","kind":"service","namespace":"home","name":"scrypted"}]}`,
			want: []string{"entities[0].provenance.source"},
		},
		{
			name: "a relation missing its target is rejected",
			in: `{"schemaVersion":"v1alpha1","relations":[{
				"from":"service:home/scrypted","type":"runs_on","provenance":{"source":"x"}}]}`,
			want: []string{"relations[0].to"},
		},
		{
			name: "a note attached to nothing is rejected",
			in: `{"schemaVersion":"v1alpha1","notes":[{
				"body":"restart it with kubectl","kind":"gotcha","provenance":{"source":"x"}}]}`,
			want: []string{"notes[0].refs"},
		},
		{
			name: "a note without a kind is rejected",
			in: `{"schemaVersion":"v1alpha1","notes":[{
				"refs":["service:home/scrypted"],"body":"restart it","provenance":{"source":"x"}}]}`,
			want: []string{"notes[0].kind"},
		},
		{
			name: "an observation without a ref is rejected",
			in:   `{"schemaVersion":"v1alpha1","observations":[{"provenance":{"source":"x"}}]}`,
			want: []string{"observations[0].ref"},
		},
		{
			name: "an empty batch is valid, since a source may legitimately be empty",
			in:   `{"schemaVersion":"v1alpha1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertViolations(t, conformance.ValidateBatch([]byte(tt.in)), tt.want)
		})
	}
}

// If this flag does not survive the wire, Dusk reads an incomplete batch as
// authoritative and deletes real entities on a transient failure.
func TestADR0011_PartialFlagSurvivesRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		partial bool
	}{
		{
			name:    "partial true survives",
			json:    `{"schemaVersion":"v1alpha1","partial":true}`,
			partial: true,
		},
		{
			name:    "partial false survives",
			json:    `{"schemaVersion":"v1alpha1","partial":false}`,
			partial: false,
		},
		{
			name:    "an omitted partial defaults to false, meaning authoritative",
			json:    `{"schemaVersion":"v1alpha1"}`,
			partial: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conformance.ValidateBatch([]byte(tt.json))
			if !got.OK() {
				t.Fatalf("want clean, got:\n%s", got.Error())
			}
			if got.Batch.GetPartial() != tt.partial {
				t.Errorf("partial = %v, want %v", got.Batch.GetPartial(), tt.partial)
			}
		})
	}
}

// The README promises a shell script is enough. If its output stops
// validating, that promise is false.
func TestADR0002_ShellPluginOutputIsAccepted(t *testing.T) {
	const jqOutput = `{
  "schema_version": "v1alpha1",
  "entities": [
    {
      "ref": "service:default/nginx",
      "kind": "service",
      "namespace": "default",
      "name": "nginx",
      "provenance": {"source": "kubectl-sh", "observed_at": "2026-08-11T15:04:05Z"}
    },
    {
      "ref": "service:kube-system/coredns",
      "kind": "service",
      "namespace": "kube-system",
      "name": "coredns",
      "provenance": {"source": "kubectl-sh", "observed_at": "2026-08-11T15:04:05Z"}
    }
  ]
}`

	got := conformance.ValidateBatch([]byte(jqOutput))
	if !got.OK() {
		t.Fatalf("documented shell example must validate, got:\n%s", got.Error())
	}
	if n := len(got.Batch.GetEntities()); n != 2 {
		t.Errorf("entities = %d, want 2", n)
	}
}

// TestADR0007_ProtojsonAcceptsBothFieldNamings guards the Tier 1 contract:
// hand-written and jq-generated JSON use snake_case, while protojson emits
// lowerCamelCase. Both must be accepted or shell plugins break.
func TestADR0007_ProtojsonAcceptsBothFieldNamings(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"snake_case as a shell script would write it", `{"schema_version":"v1alpha1"}`},
		{"lowerCamelCase as protojson emits it", `{"schemaVersion":"v1alpha1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := conformance.ValidateBatch([]byte(tt.json)); !got.OK() {
				t.Errorf("want accepted, got:\n%s", got.Error())
			}
		})
	}
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		name                    string
		ref                     string
		kind, namespace, method string
		wantErr                 bool
	}{
		{name: "a canonical ref splits into its three parts", ref: "service:home/scrypted", kind: "service", namespace: "home", method: "scrypted"},
		{name: "an empty namespace is allowed", ref: "host:/mini-1", kind: "host", namespace: "", method: "mini-1"},
		{name: "a name containing dots is allowed", ref: "host:home/mini-1.stout.zone", kind: "host", namespace: "home", method: "mini-1.stout.zone"},
		{name: "a ref with no kind prefix is rejected", ref: "scrypted", wantErr: true},
		{name: "a ref with no namespace separator is rejected", ref: "service:scrypted", wantErr: true},
		{name: "a ref with an empty name is rejected", ref: "service:home/", wantErr: true},
		{name: "a name containing a slash is rejected, since it would not round-trip", ref: "service:home/a/b", wantErr: true},
		{name: "an empty ref is rejected", ref: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ns, name, err := conformance.ParseRef(tt.ref)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRef(%q) = (%q,%q,%q), want error", tt.ref, kind, ns, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRef(%q) unexpected error: %v", tt.ref, err)
			}
			if kind != tt.kind || ns != tt.namespace || name != tt.method {
				t.Errorf("ParseRef(%q) = (%q,%q,%q), want (%q,%q,%q)",
					tt.ref, kind, ns, name, tt.kind, tt.namespace, tt.method)
			}
			if round := conformance.CanonicalRef(kind, ns, name); round != tt.ref {
				t.Errorf("round trip = %q, want %q", round, tt.ref)
			}
		})
	}
}

// TestActionDryRunUnsupportedIsExplicit guards ADR-0015: dry run is required
// of every action, and "unsupported" must be a statement rather than an
// absence, because Dusk surfaces it to a human before they approve.
func TestActionDryRunUnsupportedIsExplicit(t *testing.T) {
	tests := []struct {
		name          string
		json          string
		wantSupported bool
	}{
		{"a plugin declaring no preview support", `{"supported":false}`, false},
		{"a plugin returning a preview", `{"supported":true,"summary":"would restart 1 pod"}`, true},
		{"an empty response reads as unsupported rather than as supported", `{}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &duskv1alpha1.DryRunResponse{}
			if err := protojson.Unmarshal([]byte(tt.json), resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.GetSupported() != tt.wantSupported {
				t.Errorf("supported = %v, want %v", resp.GetSupported(), tt.wantSupported)
			}
		})
	}
}

func hasViolation(t *testing.T, r conformance.Result, path string) bool {
	t.Helper()
	for _, v := range r.Violations {
		if v.Path == path || strings.HasPrefix(v.Path, path) {
			return true
		}
	}
	return false
}
