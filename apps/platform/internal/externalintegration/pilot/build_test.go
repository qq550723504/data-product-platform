package pilot

import (
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestBackgroundAuthorityUsesCurrentServerConfiguration(t *testing.T) {
	ws, actor := uuid.New(), uuid.New()
	config := map[string]string{"EXTERNAL_INTEGRATION_API_ENABLED": "true", "EXTERNAL_INTEGRATION_API_TOKEN": "secret", "EXTERNAL_INTEGRATION_API_SUBJECT": "trusted", "EXTERNAL_INTEGRATION_API_ACTOR_ID": actor.String(), "EXTERNAL_INTEGRATION_API_WORKSPACE_IDS": ws.String()}
	auth := Authority(func(k string) string { return config[k] })
	if _, err := auth(context.Background(), ws, actor, "trusted"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth(context.Background(), ws, uuid.New(), "trusted"); err == nil {
		t.Fatal("recorded actor authenticated itself")
	}
	config["EXTERNAL_INTEGRATION_API_ENABLED"] = "false"
	if _, err := auth(context.Background(), ws, actor, "trusted"); err == nil {
		t.Fatal("background authority ignored disable")
	}
	config["EXTERNAL_INTEGRATION_API_ENABLED"] = "true"
	config["EXTERNAL_INTEGRATION_API_WORKSPACE_IDS"] = uuid.NewString()
	if _, err := auth(context.Background(), ws, actor, "trusted"); err == nil {
		t.Fatal("background authority ignored workspace revocation")
	}
}
