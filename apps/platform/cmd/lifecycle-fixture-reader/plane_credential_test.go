package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	ld "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"strings"
	"testing"
	"time"
)

func TestNativePolicyNormalizationKeepsExactVersionAndReadOnlyScope(t *testing.T) {
	cp := ld.Copy{Namespace: "closed-synthetic", Key: "fixture/b.csv", ProviderVersion: "exact-version"}
	expected := credentialPolicy(cp)
	normalized := `{"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Action":"s3:GetObjectVersion","Resource":"arn:aws:s3:::closed-synthetic/fixture/b.csv","Condition":{"StringEquals":{"s3:versionid":["exact-version"]}}}]}`
	if !sameNativePolicy(normalized, expected) {
		t.Fatal("native string/array normalization changed exact policy")
	}
	for _, bad := range []string{strings.Replace(normalized, "exact-version", "foreign-version", 1), strings.Replace(normalized, "fixture/b.csv", "fixture/*", 1), strings.Replace(normalized, "s3:GetObjectVersion", "s3:PutObject", 1), strings.Replace(normalized, `"s3:GetObjectVersion"`, `["s3:GetObjectVersion","s3:PutObject"]`, 1), `{}`} {
		if sameNativePolicy(bad, expected) {
			t.Fatal("broader/unknown native policy accepted")
		}
	}
}
func TestCredentialHashIsHashOfStableRedemptionHandle(t *testing.T) {
	p := nativeCredential{c: config{IssuerSecret: strings.Repeat("s", 40)}}
	expiry := time.Now().UTC().Add(20 * time.Minute)
	op := dd.Operation{ProviderRequestKey: "stable-request", DatasetVersionID: uuid.New(), EffectiveConsumerRef: "fixed-consumer", Action: "READ", ScopeRef: "exact", DeliveryChannel: "REDEMPTION", DeliveryMode: "PLATFORM_REDEMPTION"}
	cap := p.capability(op, expiry)
	again := p.capability(op, expiry)
	h := sha256.Sum256([]byte(cap.Credential))
	if cap != again || cap.CapabilityHash != hex.EncodeToString(h[:]) {
		t.Fatal("replay/secret hash contract mismatch")
	}
	key, secret, _ := p.keys(op.ProviderRequestKey)
	if len(key) > 20 || len(secret) > 40 {
		t.Fatal("native service account key contract exceeded")
	}
	data, _ := json.Marshal(op)
	if strings.Contains(string(data), cap.Credential) || strings.Contains(string(data), secret) {
		t.Fatal("native secret entered Core operation")
	}
}
