package verifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
)

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
			// Skip events we can't parse
			continue
		}

		// Check for access denied errors
		errorCode, _ := eventData["errorCode"].(string)
		errorMessage, _ := eventData["errorMessage"].(string)

		if strings.Contains(errorCode, "AccessDenied") || strings.Contains(errorCode, "UnauthorizedAccess") ||
			strings.Contains(errorMessage, "AccessDenied") || strings.Contains(errorMessage, "UnauthorizedAccess") {

			eventName := *event.EventName
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
