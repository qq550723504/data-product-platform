package oidc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
)

// NewVerifiedClient adds an explicit process-local CA to system roots without
// changing OS trust or disabling hostname/certificate verification.
func NewVerifiedClient(caFile string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load process certificate roots")
		}
		pem, err := os.ReadFile(caFile)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("load OIDC process CA file")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	return &http.Client{Transport: transport}, nil
}
