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
// Events are filtered to only include those containing the given filterString
// (typically the operator role prefix to scope results to a specific cluster).
func VerifyNoAccessDenied(ctx context.Context, ctClient *cloudtrail.Client, filterString string, lookback time.Duration) error {
	startTime := time.Now().Add(-lookback)

	const maxPages = 5
	var deniedEvents []string
	var nextToken *string

	for page := 0; page < maxPages; page++ {
		resp, err := ctClient.LookupEvents(ctx, &cloudtrail.LookupEventsInput{
			StartTime: &startTime,
			NextToken: nextToken,
		})
		if err != nil {
			return fmt.Errorf("looking up CloudTrail events: %w", err)
		}

		for _, event := range resp.Events {
			if event.CloudTrailEvent == nil || event.EventName == nil || event.EventTime == nil {
				continue
			}
			eventJSON := *event.CloudTrailEvent

			if filterString != "" && !strings.Contains(eventJSON, filterString) {
				continue
			}

			var eventData map[string]interface{}
			if err := json.Unmarshal([]byte(eventJSON), &eventData); err != nil {
				continue
			}

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
				deniedEvents = append(deniedEvents, fmt.Sprintf("- %s at %s: %s - %s (caller: %s)",
					eventName, eventTime.Format(time.RFC3339), errorCode, errorMessage, callerARN))
			}
		}

		nextToken = resp.NextToken
		if nextToken == nil {
			break
		}
	}

	if len(deniedEvents) > 0 {
		return fmt.Errorf("found %d AccessDenied events matching %q in the last %s:\n%s",
			len(deniedEvents), filterString, lookback, strings.Join(deniedEvents, "\n"))
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
