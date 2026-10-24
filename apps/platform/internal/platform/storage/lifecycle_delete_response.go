package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
)

// ExactDeleteResponse is safe adapter evidence, not a provider's free-form
// Message, Resource, credential, canonical request or response body. Its digest
// binds the original bounded bytes without disclosing them.
type ExactDeleteResponse struct {
	HTTPStatus   int    `json:"http_status"`
	ErrorCode    string `json:"error_code,omitempty"`
	BodySHA256   string `json:"body_sha256"`
	BodyComplete bool   `json:"body_complete"`
}

// InspectExactDeleteResponse uses a closed code vocabulary. Even an error Code
// can contain reflected credentials, so unknown strings are never persisted.
func InspectExactDeleteResponse(status int, body []byte, complete bool) ExactDeleteResponse {
	h := sha256.Sum256(body)
	r := ExactDeleteResponse{HTTPStatus: status, BodySHA256: hex.EncodeToString(h[:]), BodyComplete: complete}
	if status < 300 {
		return r
	}
	r.ErrorCode = "UNPARSEABLE_ERROR"
	if !complete {
		r.ErrorCode = "INCOMPLETE_ERROR"
		return r
	}
	var e struct {
		XMLName xml.Name `xml:"Error"`
		Codes   []string `xml:"Code"`
	}
	d := xml.NewDecoder(bytes.NewReader(body))
	var tail any
	if d.Decode(&e) != nil || d.Decode(&tail) != io.EOF || len(e.Codes) != 1 {
		return r
	}
	switch e.Codes[0] {
	case "AccessDenied", "SignatureDoesNotMatch", "InvalidAccessKeyId", "RequestTimeTooSkewed", "RequestExpired", "ExpiredToken", "InvalidToken", "AllAccessDisabled", "AuthorizationHeaderMalformed", "InvalidRequest", "MethodNotAllowed", "NoSuchKey", "NoSuchVersion", "InternalError", "ReplicationPermissionCheck", "DPPWireAdmissionDenied", "DPPWireTargetDenied", "DPPWireAttemptDenied", "DPPWireBudgetDenied":
		r.ErrorCode = e.Codes[0]
	default:
		r.ErrorCode = "UNRECOGNIZED_ERROR_CODE"
	}
	return r
}

type ExactDeleteFailure struct {
	Response ExactDeleteResponse `json:"response"`
	cause    error
}

func (e *ExactDeleteFailure) Error() string {
	return fmt.Sprintf("exact delete HTTP %d (%s): %v", e.Response.HTTPStatus, e.Response.ErrorCode, e.cause)
}
func (e *ExactDeleteFailure) Unwrap() error { return e.cause }
