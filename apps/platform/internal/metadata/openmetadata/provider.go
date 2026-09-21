package openmetadata

import metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"

// Provider is the adapter-owned provider identifier persisted in generic
// metadata binding/projection records. Core domain intentionally does not know
// concrete metadata products.
const Provider metadatadomain.Provider = "OPENMETADATA"
