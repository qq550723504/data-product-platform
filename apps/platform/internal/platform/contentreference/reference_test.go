package contentreference

import (
	"github.com/google/uuid"
	"testing"
)

func TestReferenceIdentityRequiresKnownKindAndExactInputMembership(t *testing.T) {
	for _, ref := range []Reference{{Kind: "UNKNOWN", SubjectID: uuid.New()}, {Kind: "EXECUTION_INPUT", SubjectID: uuid.New()}, {Kind: "QUALITY_RESULT", SubjectID: uuid.New(), InputName: "unexpected"}, {Kind: "DELIVERY"}} {
		if ref.Validate() == nil {
			t.Fatal("ambiguous reference accepted", ref)
		}
	}
	for _, ref := range []Reference{{Kind: "EXECUTION_INPUT", SubjectID: uuid.New(), InputName: "source"}, {Kind: "QUALITY_RESULT", SubjectID: uuid.New()}} {
		if e := ref.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	if _, ok := ForRelation("foreign", "execution_input", "dataset_version_id"); ok {
		t.Fatal("foreign schema selected managed owner")
	}
	if _, ok := ForRelation("public", "dataset_version_lineage", "output_version_id"); ok {
		t.Fatal("input owner release expanded to output history")
	}
}
