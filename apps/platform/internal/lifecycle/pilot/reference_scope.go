package pilot

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

const ReferenceEnvironment = "5d2274ee-a671-4bdc-b406-22b8dad8fd61"
const ReferenceExecution = "9ea8f3e2-c805-44b4-b133-a36f50df9d15"
const ReferencePlanHash = "d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc"
const ReferenceInventoryHash = "2b1df1ccd9e7a138bf47050cad9f031c02bd04664b64eb12aee0337ad670c9f8"

var ReferenceWorkspace = uuid.MustParse("89eba3fd-9763-4f66-8cec-1a779104f155")
var ReferenceProvider = uuid.MustParse("1980751b-c50e-4895-93a8-cb55960cd635")
var ReferenceIncarnation = uuid.MustParse("be784eda-96e4-49a9-ae47-eaa3203a2dc5")
var ReferencePlan = uuid.MustParse("8f0e9340-18f0-4a73-bdd6-622196a9011a")
var ReferenceInventoryPlan = uuid.MustParse("dfd47e35-b99a-4581-942f-96ab5d3941f1")

// Fresh values on every call; no caller-supplied locator can expand this profile.
func ReferenceCopies() []domain.Copy {
	return []domain.Copy{
		{ID: uuid.MustParse("8b1ff30c-3695-4847-b4cf-cc92cd6a399c"), WorkspaceID: ReferenceWorkspace, DatasetVersionID: uuid.MustParse("dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152"), ProviderInstanceID: ReferenceProvider, IncarnationID: ReferenceIncarnation, Namespace: "dpp-lifecycle307-online", Key: "fixture/a.csv", ProviderVersion: "312365a9-f984-43de-b9d5-7e26009ccd82", ContentSHA256: "722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0", ByteSize: 232},
		{ID: uuid.MustParse("3feb660b-21aa-4494-ada8-110cf3cf0e22"), WorkspaceID: ReferenceWorkspace, DatasetVersionID: uuid.MustParse("35b26496-f832-4496-bb91-1f50fb56e7fa"), ProviderInstanceID: ReferenceProvider, IncarnationID: ReferenceIncarnation, Namespace: "dpp-lifecycle307-online", Key: "fixture/b.csv", ProviderVersion: "159e90c3-adb6-4ea3-b2e8-2804e578e86f", ContentSHA256: "b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524", ByteSize: 129328},
		{ID: uuid.MustParse("7d85a166-59ae-4a72-a562-138e9a1617c7"), WorkspaceID: ReferenceWorkspace, DatasetVersionID: uuid.MustParse("47451712-ee38-45d1-a94e-786f59aa1647"), ProviderInstanceID: ReferenceProvider, IncarnationID: ReferenceIncarnation, Namespace: "dpp-lifecycle307-online", Key: "fixture/processed.csv", ProviderVersion: "7c8ad93b-31ee-445c-9402-816dda0410ec", ContentSHA256: "99cefa99326fb2e5daa5f23ae8802b057e5f467d5ed9813c73cdecd71f4368ad", ByteSize: 232},
		{ID: uuid.MustParse("686f6ecc-40a2-4a8f-a10b-ceaf026c6ada"), WorkspaceID: ReferenceWorkspace, DatasetVersionID: uuid.MustParse("03437e8c-9bbd-4693-87e0-fff6e651d0a5"), ProviderInstanceID: ReferenceProvider, IncarnationID: ReferenceIncarnation, Namespace: "dpp-lifecycle307-online", Key: "fixture/standardized.csv", ProviderVersion: "2139f293-87f4-4d49-a9f0-941b22275093", ContentSHA256: "044a854f6cf61dec3d77c6a907b0b30363ec5e96d5dca12649865772d7ee9163", ByteSize: 494},
		{ID: uuid.MustParse("dbe34c34-cecb-4183-a121-5eac1035d176"), WorkspaceID: ReferenceWorkspace, DatasetVersionID: uuid.MustParse("dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152"), ProviderInstanceID: ReferenceProvider, IncarnationID: ReferenceIncarnation, Namespace: "dpp-lifecycle307-quarantine", Key: "fixture/a.csv", ProviderVersion: "5a7ce790-9093-4569-a8fd-0726009a5eb4", ContentSHA256: "722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0", ByteSize: 232},
		{ID: uuid.MustParse("0a239dc6-9e1d-49eb-8604-e22a93e7b6a0"), WorkspaceID: ReferenceWorkspace, DatasetVersionID: uuid.MustParse("35b26496-f832-4496-bb91-1f50fb56e7fa"), ProviderInstanceID: ReferenceProvider, IncarnationID: ReferenceIncarnation, Namespace: "dpp-lifecycle307-quarantine", Key: "fixture/b.csv", ProviderVersion: "846d0c7c-289d-4667-bc7c-aa04e44357d4", ContentSHA256: "b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524", ByteSize: 129328},
	}
}
func ReferenceTargets() []domain.Copy {
	all := ReferenceCopies()
	out := []domain.Copy{}
	for _, c := range all {
		if (c.Namespace == "dpp-lifecycle307-online" && c.Key == "fixture/a.csv") || (c.Namespace == "dpp-lifecycle307-quarantine" && c.Key == "fixture/b.csv") {
			out = append(out, c)
		}
	}
	return out
}
