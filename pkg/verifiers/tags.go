package verifiers

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// VerifyEBSVolumesTags verifies that all EBS volumes owned by the cluster have the expected tags.
func VerifyEBSVolumesTags(ctx context.Context, ec2Client *ec2.Client, clusterID string, expectedTags map[string]string) error {
	// Filter for volumes owned by this cluster
	clusterTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", clusterID)
	filters := []types.Filter{
		{
			Name:   stringPtr("tag:" + clusterTagKey),
			Values: []string{"owned"},
		},
	}

	resp, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("describing volumes: %w", err)
	}

	if len(resp.Volumes) == 0 {
		return fmt.Errorf("no volumes found for cluster %s", clusterID)
	}

	var missingTags []string
	for _, volume := range resp.Volumes {
		volumeID := *volume.VolumeId
		volumeTags := make(map[string]string)
		for _, tag := range volume.Tags {
			volumeTags[*tag.Key] = *tag.Value
		}

		for expectedKey, expectedValue := range expectedTags {
			actualValue, found := volumeTags[expectedKey]
			if !found {
				missingTags = append(missingTags, fmt.Sprintf("volume %s missing tag %s", volumeID, expectedKey))
			} else if actualValue != expectedValue {
				missingTags = append(missingTags, fmt.Sprintf("volume %s tag %s has value %q, expected %q", volumeID, expectedKey, actualValue, expectedValue))
			}
		}
	}

	if len(missingTags) > 0 {
		return fmt.Errorf("tag validation failed:\n%s", strings.Join(missingTags, "\n"))
	}

	return nil
}

func stringPtr(s string) *string {
	return &s
}
