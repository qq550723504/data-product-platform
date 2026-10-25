package platformmanagement

import "encoding/base64"

func encodeTestCursor(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
