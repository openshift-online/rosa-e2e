package verifiers

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// VerifyEBSVolumesEncrypted verifies that all EBS volumes owned by the cluster
// are encrypted, and, when kmsKeyARN is non-empty, that they are encrypted with
// that customer-managed KMS key.
func VerifyEBSVolumesEncrypted(ctx context.Context, ec2Client *ec2.Client, clusterID, kmsKeyARN string) error {
	clusterTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", clusterID)
	resp, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		Filters: []types.Filter{
			{
				Name:   stringPtr("tag:" + clusterTagKey),
				Values: []string{"owned"},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("describing volumes: %w", err)
	}

	if len(resp.Volumes) == 0 {
		return fmt.Errorf("no volumes found for cluster %s", clusterID)
	}

	var problems []string
	for _, volume := range resp.Volumes {
		volumeID := *volume.VolumeId
		if volume.Encrypted == nil || !*volume.Encrypted {
			problems = append(problems, fmt.Sprintf("volume %s is not encrypted", volumeID))
			continue
		}
		if kmsKeyARN == "" {
			continue
		}
		volumeKey := ""
		if volume.KmsKeyId != nil {
			volumeKey = *volume.KmsKeyId
		}
		if volumeKey != kmsKeyARN {
			problems = append(problems, fmt.Sprintf("volume %s is encrypted with %q, expected the cluster KMS key %q",
				volumeID, volumeKey, kmsKeyARN))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("EBS encryption validation failed:\n%s", strings.Join(problems, "\n"))
	}

	return nil
}
