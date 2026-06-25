package verifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
)

type knownDenial struct {
	eventName   string
	rolePattern string // must appear in the caller ARN
}

// knownAccessDeniedEvents lists AccessDenied events to ignore, scoped to specific
// API actions AND caller roles to avoid masking unrelated regressions.
var knownAccessDeniedEvents = []knownDenial{
	// OCPBUGS-69881: Installer-Role missing elasticloadbalancing:DeleteListener
	{eventName: "DeleteListener", rolePattern: "Installer-Role"},
}

func isKnownDenial(eventName string, callerARN string) bool {
	for _, k := range knownAccessDeniedEvents {
		if k.eventName == eventName && strings.Contains(callerARN, k.rolePattern) {
			return true
		}
	}
	return false
}

// VerifyNoAccessDenied queries CloudTrail for AccessDenied or UnauthorizedAccess events
// within the specified lookback window and returns an error if any are found.
func VerifyNoAccessDenied(ctx context.Context, ctClient *cloudtrail.Client, clusterID string, lookback time.Duration) error {
	startTime := time.Now().Add(-lookback)

	resp, err := ctClient.LookupEvents(ctx, &cloudtrail.LookupEventsInput{
		StartTime: &startTime,
	})
	if err != nil {
		return fmt.Errorf("looking up CloudTrail events: %w", err)
	}

	var deniedEvents []string
	for _, event := range resp.Events {
		// Parse the CloudTrailEvent JSON
		var eventData map[string]interface{}
		if err := json.Unmarshal([]byte(*event.CloudTrailEvent), &eventData); err != nil {
			continue
		}

		// Check for access denied errors
		errorCode, _ := eventData["errorCode"].(string)
		errorMessage, _ := eventData["errorMessage"].(string)

		if strings.Contains(errorCode, "AccessDenied") || strings.Contains(errorCode, "UnauthorizedAccess") ||
			strings.Contains(errorMessage, "AccessDenied") || strings.Contains(errorMessage, "UnauthorizedAccess") {

			callerARN := callerARNFromEvent(eventData)
			eventName := *event.EventName
			if isKnownDenial(eventName, callerARN) {
				continue
			}

			eventTime := *event.EventTime
			deniedEvents = append(deniedEvents, fmt.Sprintf("- %s at %s: %s - %s",
				eventName, eventTime.Format(time.RFC3339), errorCode, errorMessage))
		}
	}

	if len(deniedEvents) > 0 {
		return fmt.Errorf("found %d AccessDenied events in the last %s:\n%s",
			len(deniedEvents), lookback, strings.Join(deniedEvents, "\n"))
	}

	return nil
}

func callerARNFromEvent(eventData map[string]interface{}) string {
	identity, _ := eventData["userIdentity"].(map[string]interface{})
	if identity == nil {
		return ""
	}
	if arn, ok := identity["arn"].(string); ok {
		return arn
	}
	sessionCtx, _ := identity["sessionContext"].(map[string]interface{})
	if sessionCtx == nil {
		return ""
	}
	sessionIssuer, _ := sessionCtx["sessionIssuer"].(map[string]interface{})
	if sessionIssuer == nil {
		return ""
	}
	arn, _ := sessionIssuer["arn"].(string)
	return arn
}
