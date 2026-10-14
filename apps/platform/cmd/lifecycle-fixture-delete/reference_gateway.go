package main

import (
	"encoding/json"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"strings"
)

func referenceIAMPolicy() []byte {
	statements := []any{}
	for _, c := range pilot.ReferenceTargets() {
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:DeleteObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + c.Namespace + "/" + c.Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": c.ProviderVersion}}})
	}
	b, _ := json.MarshalIndent(map[string]any{"Version": "2012-10-17", "Statement": statements}, "", "  ")
	return b
}
func referenceGateway(deleteAccess, inspectAccess string, active bool) string {
	var b strings.Builder
	b.WriteString("{\n admin off\n auto_https off\n}\n:9000 {\n route {\n  @bypass header X-Amz-Bypass-Governance-Retention *\n  respond @bypass 403\n  @copy header X-Amz-Copy-Source *\n  respond @copy 403\n  @unsigned header X-Amz-Content-Sha256 STREAMING-UNSIGNED-PAYLOAD-TRAILER\n  respond @unsigned 403\n")
	if active {
		targets := []string{}
		for _, c := range pilot.ReferenceTargets() {
			targets = append(targets, "{http.request.uri} == '/"+c.Namespace+"/"+c.Key+"?versionId="+c.ProviderVersion+"'")
		}
		b.WriteString("  @approvedDelete {\n   method DELETE\n   expression {http.request.header.Authorization}.contains('Credential=" + deleteAccess + "/') && (" + strings.Join(targets, " || ") + ")\n  }\n  reverse_proxy @approvedDelete dpp-lifecycle307-5d2274ee-minio:9000 {\n   @accepted status 204\n   handle_response @accepted {\n    abort\n   }\n  }\n")
	}
	b.WriteString("  @mutate method DELETE PUT POST PATCH\n  respond @mutate 403\n  @payload method GET\n")
	// Metadata operations are restricted to the inspector identity. Payload GETs
	// (no retention/legal-hold/versions parameter) remain denied on this gateway.
	b.WriteString("  @metadata {\n   method GET HEAD\n   expression {http.request.header.Authorization}.contains('Credential=" + inspectAccess + "/')\n  }\n  route @metadata {\n   @heads method HEAD\n   reverse_proxy @heads dpp-lifecycle307-5d2274ee-minio:9000\n   @retention query retention=*\n   reverse_proxy @retention dpp-lifecycle307-5d2274ee-minio:9000\n   @hold query legal-hold=*\n   reverse_proxy @hold dpp-lifecycle307-5d2274ee-minio:9000\n   @versions {\n    method GET\n    path /dpp-lifecycle307-online /dpp-lifecycle307-online/ /dpp-lifecycle307-quarantine /dpp-lifecycle307-quarantine/\n    query versions=*\n   }\n   reverse_proxy @versions dpp-lifecycle307-5d2274ee-minio:9000\n   respond 403\n  }\n  respond 403\n }\n}\n")
	return b.String()
}
