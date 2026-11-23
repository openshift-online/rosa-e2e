package reporting

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	ocmerrors "github.com/openshift-online/ocm-sdk-go/errors"
)

// The OCM SDK generic request API (sdk.Connection.Get().Path(...).SendContext(ctx)) treats every
// HTTP status as a valid response: *sdk.Response exposes only Status(), Bytes()/String() and
// Header(name), and a non-2xx status is never turned into an error. Asserting on Status() alone
// therefore produces failures that say nothing beyond the numeric code, so the helpers below
// rebuild the missing context from what the SDK actually offers.
//
// Response bodies may contain account names, free-form reasons, error pages or credentials, so
// the body is parsed with the SDK's own errors.UnmarshalError and only two fields are considered:
// "code" and "operation_id". Reasons, details, timestamps, hrefs, headers and query strings are
// dropped and never reach the output.
//
// Limitation, stated plainly: a value matching its pattern is emitted as received. The patterns
// bound shape and length only. Nothing here inspects content, classifies it or detects secrets,
// so a matching value is emitted whatever it turns out to contain. Emitting these identifiers is
// the point of the file: without them a failed assertion cannot be correlated with a server-side
// record. Everything that does not match is replaced by a constant.
//
// The SDK offers no other machine identifier for a generic response. There is no typed accessor
// for a correlation header, so when a response carries no parseable OCM error document the
// diagnostic is limited to the endpoint and the status.
const (
	// ocmErrorBodyLimit is a conservative cap on how much of a response body is handed to the
	// JSON parser. OCM error documents are a few hundred bytes, so this leaves ample headroom.
	// A larger body is reported as having no identifiers; the code makes no assumption about
	// what such a body is or where it came from.
	ocmErrorBodyLimit = 8 * 1024

	// diagnosticUnavailable is the placeholder printed for any field that is absent or fails the
	// allowlist. It is a constant in this file, never server data.
	diagnosticUnavailable = "unavailable"

	// ocmAccessTokenErrorPrefix is the fixed prefix the SDK authentication transport wrapper adds
	// when it cannot obtain an access token. Only its presence is used, never its content.
	ocmAccessTokenErrorPrefix = "can't get access token"
)

var (
	// Shape check for a static API route such as /api/osd_fleet_mgmt/v1/management_clusters. It
	// constrains the value to slash-separated word segments, so a query string, a fragment, an
	// absolute URL or embedded whitespace cannot match. It is a bound on shape, not proof that
	// every accepted value is a real route: what guarantees that is the caller contract, since
	// the only callers pass the package-level path constants declared next to the specs.
	ocmEndpointPattern = regexp.MustCompile(`^(/[A-Za-z0-9_.-]{1,63}){1,10}$`)

	// OCM error codes are upper-case machine identifiers of at most four dash-separated segments,
	// e.g. CLUSTERS-MGMT-401, ACCT-MGMT-11, OSDFM-403. Bounded to 64 characters.
	ocmErrorCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}(-[A-Z0-9]{1,15}){0,3}$`)

	// Only the decimal form the SDK documents is accepted ("operation_id": "123", at
	// authentication/handler.go:234-244). Any other value, of any length, renders as
	// diagnosticUnavailable. The 16-digit ceiling is a conservative cap chosen here, not a
	// length the SDK guarantees.
	ocmOperationIDPattern = regexp.MustCompile(`^[0-9]{1,16}$`)
)

// Transport failure classes. The underlying error text is never reported because OCM SDK transport
// errors can embed the request URL, its query string, or a token endpoint response body.
const (
	transportFailureNone        = "none"
	transportFailureCanceled    = "context-canceled"
	transportFailureDeadline    = "context-deadline-exceeded"
	transportFailureAuth        = "authentication"
	transportFailureDNS         = "dns"
	transportFailureTimeout     = "timeout"
	transportFailureTLS         = "tls"
	transportFailureConnection  = "connection"
	transportFailureUnspecified = "unclassified"
)

// DescribeOCMResponse renders a single-line description of an unexpected OCM API response for use
// as an assertion failure annotation. endpoint is the static request path, status is
// sdk.Response.Status(), and body is sdk.Response.Bytes(), which may be nil. Every field is passed
// through its allowlist, so anything absent or rejected renders as diagnosticUnavailable.
func DescribeOCMResponse(endpoint string, status int, body []byte) string {
	code, operationID := ocmErrorIdentifiers(body)
	return fmt.Sprintf(
		"unexpected OCM API response: endpoint=%s status=%s code=%s operation_id=%s",
		orUnavailable(safeEndpoint(endpoint)),
		safeHTTPStatusText(status),
		orUnavailable(safeOCMErrorCode(code)),
		orUnavailable(safeOCMOperationID(operationID)),
	)
}

// DescribeOCMTransportError renders a single-line description of a failure to send an OCM API
// request. Only the static endpoint and a fixed failure class are reported. A nil error yields the
// "none" class so the function is total; callers are expected to use it only on the failure path.
func DescribeOCMTransportError(endpoint string, err error) string {
	return fmt.Sprintf(
		"OCM API request failed before a response was received: endpoint=%s failure=%s",
		orUnavailable(safeEndpoint(endpoint)), classifyTransportError(err),
	)
}

// ocmErrorIdentifiers parses an OCM error document and returns its raw code and operation
// identifier. Both are empty unless the whole document parsed cleanly.
func ocmErrorIdentifiers(body []byte) (code, operationID string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || len(trimmed) > ocmErrorBodyLimit {
		return "", ""
	}
	// Only a JSON object can be an OCM error document, so anything else is not parsed.
	if trimmed[0] != '{' {
		return "", ""
	}
	// ocmerrors.UnmarshalError reads the first JSON value and returns, so it never inspects what
	// follows it: `{"code":"X-1","operation_id":"1"}garbage` parses without error and yields both
	// identifiers. Validate the whole document first so only a body that is entirely one JSON
	// value reaches the parser, which is what the contract above promises.
	if !json.Valid(trimmed) {
		return "", ""
	}
	parsed, err := ocmerrors.UnmarshalError(trimmed)
	if err != nil || parsed == nil {
		return "", ""
	}
	return parsed.Code(), parsed.OperationID()
}

func safeEndpoint(value string) string {
	if ocmEndpointPattern.MatchString(value) {
		return value
	}
	return ""
}

// safeHTTPStatusText renders a status only when it is in the range of real HTTP status codes.
func safeHTTPStatusText(status int) string {
	if status < 100 || status > 599 {
		return diagnosticUnavailable
	}
	return strconv.Itoa(status)
}

func safeOCMErrorCode(value string) string {
	if ocmErrorCodePattern.MatchString(value) {
		return value
	}
	return ""
}

func safeOCMOperationID(value string) string {
	if ocmOperationIDPattern.MatchString(value) {
		return value
	}
	return ""
}

// classifyTransportError maps a send failure to one of the fixed classes above. Cancellation is
// checked first because it is unambiguous, then token acquisition: the SDK sends requests straight
// through transport.RoundTrip, so an expired OCM_TOKEN surfaces as a plain wrapped error with no
// net or tls type in its chain and would otherwise be reported as unclassified.
func classifyTransportError(err error) string {
	switch {
	case err == nil:
		return transportFailureNone
	case errors.Is(err, context.Canceled):
		return transportFailureCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return transportFailureDeadline
	case strings.HasPrefix(err.Error(), ocmAccessTokenErrorPrefix):
		return transportFailureAuth
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return transportFailureDNS
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return transportFailureTimeout
	}

	var certErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &recordErr) {
		return transportFailureTLS
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return transportFailureConnection
	}

	return transportFailureUnspecified
}

func orUnavailable(value string) string {
	if value == "" {
		return diagnosticUnavailable
	}
	return value
}
