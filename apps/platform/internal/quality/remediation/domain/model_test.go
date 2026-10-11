package domain

import (
	"github.com/google/uuid"
	"testing"
)

func TestManualHandlingCannotMintAuthority(t *testing.T) {
	actor := uuid.New()
	if CanHandle(false, actor, uuid.New()) != ErrAssignee {
		t.Fatal("foreign handler accepted")
	}
	if CanHandle(true, actor, actor) != ErrClosed {
		t.Fatal("manual closure allowed more work")
	}
	if ValidateCommand(uuid.Nil, actor, "key") != ErrScope {
		t.Fatal("unknown workspace accepted")
	}
	if ValidateReason(" ") == nil {
		t.Fatal("reason missing")
	}
	if err := CanHandle(false, actor, actor); err != nil {
		t.Fatal(err)
	}
}
