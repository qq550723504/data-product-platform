package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProtectedCopyRequiresWorkspaceKeyAndCreationWitness(t *testing.T) {
	w := uuid.New()
	c := ProtectedCopy{ID: uuid.New(), WorkspaceID: w, DatasetVersionID: uuid.New(), ProviderInstanceID: uuid.New(), IncarnationID: uuid.New(), VerificationID: uuid.New(), Namespace: "raw", Key: "core/" + w.String() + "/datasets/result.csv", ContentSHA256: strings.Repeat("a", 64), ByteSize: 20}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"datasets/result.csv", "core/" + uuid.NewString() + "/result.csv", "core/" + w.String() + "/../other.csv", "core/" + w.String() + "/%2e%2e/other.csv"} {
		x := c
		x.Key = key
		if x.Validate() == nil {
			t.Fatalf("unqualified physical identity accepted: %q", key)
		}
	}
	c.VerificationID = uuid.Nil
	if c.Validate() == nil {
		t.Fatal("hash/key alone claimed verified creation")
	}
}
