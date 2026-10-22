package storage

import (
	"context"
	"fmt"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"net/http"
	"strings"
	"testing"
)

func TestCopyRecoveryCompleteInventoryAndTaggedExactHead(t *testing.T) {
	for _, scenario := range []string{"present", "missing", "ambiguous", "marker", "truncated", "wrong-tag", "wrong-size", "head-denied"} {
		t.Run(scenario, func(t *testing.T) {
			o := copyFixture()
			j := &inspectionTestJournal{}
			requests := 0
			copier := copierTest(t, o, j, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method == http.MethodGet && r.URL.Query().Has("versions") {
					version := fmt.Sprintf(`<Version><Key>%s</Key><VersionId>target-version</VersionId><IsLatest>true</IsLatest><LastModified>2026-10-11T10:00:00Z</LastModified><ETag>fixture</ETag><Size>%d</Size></Version>`, o.TargetKey, o.Source.ByteSize)
					switch scenario {
					case "missing":
						version = ""
					case "ambiguous":
						version += strings.ReplaceAll(version, "target-version", "other-version")
					case "marker":
						version = fmt.Sprintf(`<DeleteMarker><Key>%s</Key><VersionId>target-version</VersionId></DeleteMarker>`, o.TargetKey)
					}
					truncated := "false"
					if scenario == "truncated" {
						truncated = "true"
					}
					fmt.Fprintf(w, `<ListVersionsResult><Name>%s</Name><Prefix></Prefix><IsTruncated>%s</IsTruncated>%s</ListVersionsResult>`, o.TargetNamespace, truncated, version)
					return
				}
				if r.Method != http.MethodHead || r.URL.Query().Get("versionId") != "target-version" {
					t.Error("non read-only or unversioned lookup")
					w.WriteHeader(403)
					return
				}
				if scenario == "head-denied" {
					w.WriteHeader(403)
					return
				}
				c := o.Materialized("target-version")
				if scenario == "wrong-size" {
					c.ByteSize++
				}
				tag := o.ID.String()
				if scenario == "wrong-tag" {
					tag = "00000000-0000-0000-0000-000000000001"
				}
				w.Header().Set("X-Amz-Meta-Lifecycle-Operation", tag)
				exactStatResponse(w, c)
			})
			recovery := &LifecycleCopyRecovery{copier: copier}
			p, e := recovery.DiscoverCopyTarget(context.Background(), o)
			if scenario == "present" {
				if e != nil || p.Copy != o.Materialized("target-version") || p.OperationTag != o.ID || requests != 2 || j.observations[1].CopyOperationTag != o.ID {
					t.Fatalf("tagged recovery: %v %+v", e, p)
				}
			} else if e == nil {
				t.Fatal("uncertain inventory or HEAD accepted")
			}
			if scenario == "missing" || scenario == "ambiguous" || scenario == "marker" || scenario == "truncated" {
				if requests != 1 {
					t.Fatal("uncertain closure caused extra I/O")
				}
			}
			for _, c := range j.starts {
				if c.Operation != lc.StorageRecoverCopyVersions && c.Operation != lc.StorageRecoverCopyExact {
					t.Fatal("recovery admitted mutation")
				}
			}
		})
	}
}
func TestNativeCopyResponseLossIsOneInvocationWithoutAck(t *testing.T) {
	o := copyFixture()
	j := &inspectionTestJournal{}
	requests := 0
	s := copierTest(t, o, j, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("X-Amz-Version-Id", "native-version")
		fmt.Fprint(w, `<CopyObjectResult><ETag>fixture</ETag><LastModified>2026-10-11T10:00:00Z</LastModified></CopyObjectResult>`)
	})
	s.transport.base = copyResponseLoss{s.transport.base}
	if _, _, e := s.CreateCopy(context.Background(), o); e == nil {
		t.Fatal("lost response returned success")
	}
	if requests != 1 || len(j.starts) != 1 || len(j.observations) != 1 || j.observations[0].InvokedAt == nil || j.observations[0].HTTPStatus != 0 || j.observations[0].CopyResultVersion != "" || j.observations[0].Failure != "TRANSPORT" {
		t.Fatal("response loss fabricated ACK or resent COPY")
	}
}
