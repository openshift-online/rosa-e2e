package reporting

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

// Tokens that must never reach a CI log or JUnit report. Every case below feeds at least one of
// them to the helpers and then asserts the rendered output is free of all of them. The hyphenated
// forms matter: they are shapes a naive identifier allowlist would wave through.
var forbiddenDiagnosticTokens = []string{
	"sha256~bearer-token-must-not-be-captured",
	"bearer-token-must-not-be-captured",
	"Bearer",
	"account-id-must-not-be-captured",
	"reason-must-not-be-captured",
	"<html>",
	"<title>",
	"api.example.internal",
	"search=",
	"status='ready'",
	"x-secret-header",
}

func assertNoForbiddenTokens(t *testing.T, rendered string) {
	t.Helper()
	for _, token := range forbiddenDiagnosticTokens {
		if strings.Contains(rendered, token) {
			t.Fatalf("rendered diagnostic leaked %q: %s", token, rendered)
		}
	}
	if strings.ContainsAny(rendered, "\n\r\x00") {
		t.Fatalf("rendered diagnostic is not a single line of printable text: %q", rendered)
	}
}

// ocmErrorBody builds an OCM error document in the shape the services actually return, including
// the free-form fields that must be dropped.
func ocmErrorBody(t *testing.T, code, operationID string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"kind":         "Error",
		"id":           "401",
		"href":         "/api/osd_fleet_mgmt/v1/errors/401",
		"code":         code,
		"reason":       "reason-must-not-be-captured for account-id-must-not-be-captured",
		"operation_id": operationID,
		"details": map[string]any{
			"token": "sha256~bearer-token-must-not-be-captured",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func wantDiagnostic(endpoint, status, code, operationID string) string {
	return fmt.Sprintf(
		"unexpected OCM API response: endpoint=%s status=%s code=%s operation_id=%s",
		endpoint, status, code, operationID,
	)
}

func TestDescribeOCMResponseReportsAllowlistedIdentifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		code        string
		operationID string
	}{
		// Codes and the operation id shape the SDK documents at authentication/handler.go:234-244.
		{name: "short code", code: "OSDFM-403", operationID: "9007199254"},
		{name: "long dash separated code", code: "CLUSTERS-MGMT-401", operationID: "4815162342"},
		{name: "documented decimal counter", code: "ACCOUNTS-MGMT-401", operationID: "123"},
		{name: "numeric suffix code", code: "CLUSTERS-MGMT-11", operationID: "456789"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			endpoint := "/api/osd_fleet_mgmt/v1/management_clusters"
			rendered := DescribeOCMResponse(endpoint, 401, ocmErrorBody(t, test.code, test.operationID))

			want := wantDiagnostic(endpoint, "401", test.code, test.operationID)
			if rendered != want {
				t.Fatalf("got %q, want %q", rendered, want)
			}
			assertNoForbiddenTokens(t, rendered)
		})
	}
}

func TestDescribeOCMResponseSuccessStatus(t *testing.T) {
	t.Parallel()

	// A 200 carries a normal payload, not an error document, so no identifiers are reported.
	endpoint := "/api/osd_fleet_mgmt/v1/service_clusters"
	rendered := DescribeOCMResponse(endpoint, 200, []byte(`{"kind":"ServiceClusterList","page":1,"size":1,"total":3}`))

	want := wantDiagnostic(endpoint, "200", "unavailable", "unavailable")
	if rendered != want {
		t.Fatalf("got %q, want %q", rendered, want)
	}
}

func TestDescribeOCMResponseForbiddenKeepsIdentifiers(t *testing.T) {
	t.Parallel()

	// 403 is skipped rather than failed by the e2e spec, but the helper must still render it
	// consistently when it is used for reporting.
	endpoint := "/api/osd_fleet_mgmt/v1/service_clusters"
	rendered := DescribeOCMResponse(endpoint, 403, ocmErrorBody(t, "OSDFM-403", "9007199254"))

	want := wantDiagnostic(endpoint, "403", "OSDFM-403", "9007199254")
	if rendered != want {
		t.Fatalf("got %q, want %q", rendered, want)
	}
	assertNoForbiddenTokens(t, rendered)
}

func TestDescribeOCMResponseRejectsUnsafeInput(t *testing.T) {
	t.Parallel()

	const serviceClusters = "/api/osd_fleet_mgmt/v1/service_clusters"
	const managementClusters = "/api/osd_fleet_mgmt/v1/management_clusters"

	htmlBody := []byte(
		"<html><head><title>401 Authorization Required</title></head>" +
			"<body>account-id-must-not-be-captured</body></html>",
	)

	// Exactly one byte over the parser limit, built from a well-formed error document so the only
	// reason its identifiers are dropped is the size bound.
	oversizedBody := ocmErrorBody(t, "OSDFM-500", "4815162342")
	padding := ocmErrorBodyLimit + 1 - len(oversizedBody) - len(`,"padding":""`)
	oversizedBody = append(
		oversizedBody[:len(oversizedBody)-1],
		[]byte(`,"padding":"`+strings.Repeat("x", padding)+`"}`)...,
	)
	if len(oversizedBody) != ocmErrorBodyLimit+1 {
		t.Fatalf("oversized body is %d bytes, want %d", len(oversizedBody), ocmErrorBodyLimit+1)
	}

	tests := []struct {
		name            string
		endpoint        string
		status          int
		body            []byte
		wantEndpoint    string
		wantStatus      string
		wantCode        string
		wantOperationID string
	}{
		{
			name:     "html error page is not parsed",
			endpoint: managementClusters,
			status:   401,
			body:     htmlBody,

			wantEndpoint: managementClusters, wantStatus: "401",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			name:     "truncated json yields no identifiers",
			endpoint: managementClusters,
			status:   502,
			body: []byte(
				`{"kind":"Error","code":"OSDFM-502","reason":"reason-must-not-be-captured`,
			),

			wantEndpoint: managementClusters, wantStatus: "502",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			name:     "empty body yields no identifiers",
			endpoint: serviceClusters,
			status:   504,
			body:     nil,

			wantEndpoint: serviceClusters, wantStatus: "504",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			name:     "body one byte over the limit is not parsed",
			endpoint: serviceClusters,
			status:   500,
			body:     oversizedBody,

			wantEndpoint: serviceClusters, wantStatus: "500",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			name:     "free-form code and operation id are dropped",
			endpoint: serviceClusters,
			status:   401,
			body: []byte(
				`{"code":"reason-must-not-be-captured",` +
					`"operation_id":"Bearer sha256~bearer-token-must-not-be-captured"}`,
			),

			wantEndpoint: serviceClusters, wantStatus: "401",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			// Regression guard: a hyphenated word is exactly the shape a loose identifier
			// allowlist would accept, so it must be rejected on its own merits.
			name:     "hyphenated word is not accepted as an operation id",
			endpoint: serviceClusters,
			status:   401,
			body: []byte(
				`{"code":"OSDFM-401","operation_id":"account-id-must-not-be-captured"}`,
			),

			wantEndpoint: serviceClusters, wantStatus: "401",
			wantCode: "OSDFM-401", wantOperationID: "unavailable",
		},
		{
			// Escaped control characters survive JSON parsing, so the allowlist is the only thing
			// keeping a NUL out of a JUnit XML attribute.
			name:     "escaped control characters are dropped",
			endpoint: serviceClusters,
			status:   500,
			body:     []byte(`{"code":"OSDFM\u0000500","operation_id":"1k0JQ9KKYjSd\u0000KxLRCSHSDQ"}`),

			wantEndpoint: serviceClusters, wantStatus: "500",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			// Not a decimal, so it is dropped like any other non-matching value. See
			// TestOperationIDAllowlistBounds for the full boundary table.
			name:     "hex and dash identifier is not accepted as an operation id",
			endpoint: serviceClusters,
			status:   401,
			body:     []byte(`{"code":"OSDFM-401","operation_id":"f81d4fae-7dec-11d0-a765-00a0c91e6bf6"}`),

			wantEndpoint: serviceClusters, wantStatus: "401",
			wantCode: "OSDFM-401", wantOperationID: "unavailable",
		},
		{
			name:     "lower case code is dropped",
			endpoint: serviceClusters,
			status:   401,
			body:     []byte(`{"code":"osdfm-401","operation_id":"4815162342"}`),

			wantEndpoint: serviceClusters, wantStatus: "401",
			wantCode: "unavailable", wantOperationID: "4815162342",
		},
		{
			name:         "endpoint carrying a query string is dropped",
			endpoint:     "https://api.example.internal/api/osd_fleet_mgmt/v1/management_clusters?search=status='ready'",
			status:       401,
			body:         ocmErrorBody(t, "OSDFM-401", "4815162342"),
			wantEndpoint: "unavailable", wantStatus: "401",
			wantCode: "OSDFM-401", wantOperationID: "4815162342",
		},
		{
			name:     "status below the http range is dropped",
			endpoint: serviceClusters,
			status:   0,
			body:     nil,

			wantEndpoint: serviceClusters, wantStatus: "unavailable",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
		{
			name:     "status above the http range is dropped",
			endpoint: serviceClusters,
			status:   600,
			body:     nil,

			wantEndpoint: serviceClusters, wantStatus: "unavailable",
			wantCode: "unavailable", wantOperationID: "unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rendered := DescribeOCMResponse(test.endpoint, test.status, test.body)

			want := wantDiagnostic(test.wantEndpoint, test.wantStatus, test.wantCode, test.wantOperationID)
			if rendered != want {
				t.Fatalf("got %q, want %q", rendered, want)
			}
			assertNoForbiddenTokens(t, rendered)
		})
	}
}

// TestWrongTypedIdentifierFieldsAreDropped covers error documents where "code" or
// "operation_id" is not a JSON string. The identifiers must be dropped rather than coerced,
// while the endpoint and status still render.
func TestWrongTypedIdentifierFieldsAreDropped(t *testing.T) {
	t.Parallel()

	const endpoint = "/api/osd_fleet_mgmt/v1/management_clusters"

	tests := []struct {
		name string
		body string
	}{
		{name: "numeric code and operation id", body: `{"code":401,"operation_id":123}`},
		{name: "boolean code", body: `{"code":true,"operation_id":"123"}`},
		{name: "null code and operation id", body: `{"code":null,"operation_id":null}`},
		{name: "array code", body: `{"code":["OSDFM-401"],"operation_id":"123"}`},
		{name: "object operation id", body: `{"code":"OSDFM-401","operation_id":{"v":"123"}}`},
		{name: "json array document", body: `[{"code":"OSDFM-401","operation_id":"123"}]`},
		{name: "json string document", body: `"OSDFM-401"`},
		{name: "bare number document", body: `401`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			want := wantDiagnostic(endpoint, "401", "unavailable", "unavailable")
			if got := DescribeOCMResponse(endpoint, 401, []byte(test.body)); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

// TestOperationIDAllowlistBounds pins the operation id allowlist through the real
// DescribeOCMResponse path. One shape is accepted, a decimal of 1 to 16 digits; everything
// longer, and everything containing a non-digit, renders as the placeholder.
func TestOperationIDAllowlistBounds(t *testing.T) {
	t.Parallel()

	const endpoint = "/api/osd_fleet_mgmt/v1/service_clusters"

	// repeat builds a synthetic identifier of exactly n characters from the given unit.
	repeat := func(unit string, n int) string {
		return strings.Repeat(unit, n/len(unit)+1)[:n]
	}

	tests := []struct {
		name         string
		operationID  string
		wantAccepted bool
	}{
		{name: "1 digit", operationID: repeat("1", 1), wantAccepted: true},
		{name: "16 digits upper bound of decimal form", operationID: repeat("1", 16), wantAccepted: true},
		{name: "17 digits", operationID: repeat("1", 17), wantAccepted: false},
		{name: "20 digits", operationID: repeat("1", 20), wantAccepted: false},
		{name: "26 digits", operationID: repeat("1", 26), wantAccepted: false},
		{name: "27 digits", operationID: repeat("1", 27), wantAccepted: false},
		{name: "28 digits", operationID: repeat("1", 28), wantAccepted: false},
		{name: "32 digits", operationID: repeat("1", 32), wantAccepted: false},
		{name: "33 digits", operationID: repeat("1", 33), wantAccepted: false},

		// Non-decimal input is rejected at every length.
		{name: "17 alphanumerics", operationID: repeat("aB3", 17), wantAccepted: false},
		{name: "20 alphanumerics", operationID: repeat("aB3", 20), wantAccepted: false},
		{name: "26 alphanumerics", operationID: repeat("aB3", 26), wantAccepted: false},
		{name: "27 alphanumerics", operationID: repeat("aB3", 27), wantAccepted: false},
		{name: "28 alphanumerics", operationID: repeat("aB3", 28), wantAccepted: false},
		{name: "32 alphanumerics", operationID: repeat("aB3", 32), wantAccepted: false},
		{name: "33 alphanumerics", operationID: repeat("aB3", 33), wantAccepted: false},

		// Short non-decimal values are rejected too, so the length cap is not the only guard.
		{name: "single letter", operationID: "a", wantAccepted: false},
		{name: "digits with a hyphen", operationID: "123-456", wantAccepted: false},
		{name: "digits with a space", operationID: "123 456", wantAccepted: false},
		{name: "digits with a leading plus", operationID: "+123456", wantAccepted: false},
		{name: "empty", operationID: "", wantAccepted: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			body, err := json.Marshal(map[string]any{
				"code":         "OSDFM-401",
				"operation_id": test.operationID,
			})
			if err != nil {
				t.Fatal(err)
			}

			wantOperationID := diagnosticUnavailable
			if test.wantAccepted {
				wantOperationID = test.operationID
			}
			want := wantDiagnostic(endpoint, "401", "OSDFM-401", wantOperationID)

			if got := DescribeOCMResponse(endpoint, 401, body); got != want {
				t.Fatalf("operation id %q (%d chars): got %q, want %q",
					test.operationID, len(test.operationID), got, want)
			}
		})
	}
}

// timeoutError is a net.Error that reports a timeout without being one of the concrete net types.
type timeoutError struct{}

func (timeoutError) Error() string   { return "sha256~bearer-token-must-not-be-captured" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func TestDescribeOCMTransportError(t *testing.T) {
	t.Parallel()

	// A transport error shaped like the ones the SDK surfaces: it embeds the full request URL,
	// including the search query, which must not reach the failure message.
	leakyURLError := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New(
			"Get \"https://api.example.internal/api/osd_fleet_mgmt/v1/management_clusters" +
				"?search=status='ready'\": x-secret-header sha256~bearer-token-must-not-be-captured",
		),
	}

	tests := []struct {
		name        string
		err         error
		wantFailure string
	}{
		{name: "no error", err: nil, wantFailure: "none"},
		{name: "canceled", err: fmt.Errorf("send: %w", context.Canceled), wantFailure: "context-canceled"},
		{
			name:        "deadline exceeded",
			err:         fmt.Errorf("send: %w", context.DeadlineExceeded),
			wantFailure: "context-deadline-exceeded",
		},
		{
			// The SDK authentication transport wrapper adds this prefix; an expired OCM_TOKEN has
			// no net or tls type in its chain and would otherwise be unclassified.
			name: "access token failure",
			err: fmt.Errorf("can't get access token: %w",
				errors.New("invalid_grant for account-id-must-not-be-captured")),
			wantFailure: "authentication",
		},
		{
			name:        "dns failure",
			err:         fmt.Errorf("send: %w", &net.DNSError{Err: "no such host", Name: "api.example.internal"}),
			wantFailure: "dns",
		},
		{
			name: "dns timeout is still a dns failure",
			err: fmt.Errorf("send: %w",
				&net.DNSError{Err: "i/o timeout", IsTimeout: true, Name: "api.example.internal"}),
			wantFailure: "dns",
		},
		{name: "generic timeout", err: fmt.Errorf("send: %w", timeoutError{}), wantFailure: "timeout"},
		{
			name:        "tls verification failure",
			err:         fmt.Errorf("send: %w", &tls.CertificateVerificationError{Err: errors.New("x509: unknown authority")}),
			wantFailure: "tls",
		},
		{name: "connection failure", err: fmt.Errorf("send: %w", leakyURLError), wantFailure: "connection"},
		{
			name:        "unknown failure",
			err:         errors.New("sha256~bearer-token-must-not-be-captured"),
			wantFailure: "unclassified",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rendered := DescribeOCMTransportError("/api/osd_fleet_mgmt/v1/management_clusters", test.err)

			want := "OCM API request failed before a response was received: " +
				"endpoint=/api/osd_fleet_mgmt/v1/management_clusters failure=" + test.wantFailure
			if rendered != want {
				t.Fatalf("got %q, want %q", rendered, want)
			}
			assertNoForbiddenTokens(t, rendered)
		})
	}
}

func TestDescribeOCMTransportErrorRejectsUnsafeEndpoint(t *testing.T) {
	t.Parallel()

	rendered := DescribeOCMTransportError(
		"https://api.example.internal/api/osd_fleet_mgmt/v1/management_clusters?search=status='ready'",
		errors.New("boom"),
	)

	want := "OCM API request failed before a response was received: " +
		"endpoint=unavailable failure=unclassified"
	if rendered != want {
		t.Fatalf("got %q, want %q", rendered, want)
	}
	assertNoForbiddenTokens(t, rendered)
}
