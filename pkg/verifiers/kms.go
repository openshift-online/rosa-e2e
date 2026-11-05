package verifiers

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
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
		if kmsKeyARN == "" || !isRootVolume(volume) {
			// Persistent volumes may use a per-storage-class KMS key, so only
			// the node root volumes are required to use the cluster key.
			continue
		}
		volumeKey := ""
		if volume.KmsKeyId != nil {
			volumeKey = *volume.KmsKeyId
		}
		if volumeKey != kmsKeyARN {
			problems = append(problems, fmt.Sprintf("root volume %s is encrypted with %q, expected the cluster KMS key %q",
				volumeID, volumeKey, kmsKeyARN))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("EBS encryption validation failed:\n%s", strings.Join(problems, "\n"))
	}

	return nil
}

// isRootVolume reports whether the volume is attached as a node root volume.
// Classic workers attach their root volume as /dev/sda1, HCP workers as /dev/xvda.
func isRootVolume(volume types.Volume) bool {
	for _, attachment := range volume.Attachments {
		if attachment.Device == nil {
			continue
		}
		if *attachment.Device == "/dev/sda1" || *attachment.Device == "/dev/xvda" {
			return true
		}
	}
	return false
}

// VerifyEBSVolumeEncryptedWithKey verifies that a single EBS volume is
// encrypted with the given KMS key.
func VerifyEBSVolumeEncryptedWithKey(ctx context.Context, ec2Client *ec2.Client, volumeID, kmsKeyARN string) error {
	resp, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: []string{volumeID},
	})
	if err != nil {
		return fmt.Errorf("describing volume %s: %w", volumeID, err)
	}
	if len(resp.Volumes) == 0 {
		return fmt.Errorf("volume %s not found", volumeID)
	}

	volume := resp.Volumes[0]
	if volume.Encrypted == nil || !*volume.Encrypted {
		return fmt.Errorf("volume %s is not encrypted", volumeID)
	}
	volumeKey := ""
	if volume.KmsKeyId != nil {
		volumeKey = *volume.KmsKeyId
	}
	if volumeKey != kmsKeyARN {
		return fmt.Errorf("volume %s is encrypted with %q, expected KMS key %q",
			volumeID, volumeKey, kmsKeyARN)
	}
	return nil
}

// VerifyKMSKeyHealthy verifies that the cluster's KMS key is enabled, is not
// scheduled for deletion, is an encryption key, and that its key policy
// grants the cluster's operator roles.
func VerifyKMSKeyHealthy(ctx context.Context, kmsClient *kms.Client, keyARN, operatorRolePrefix string) error {
	keyResp, err := kmsClient.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: &keyARN})
	if err != nil {
		return fmt.Errorf("describing KMS key: %w", err)
	}
	metadata := keyResp.KeyMetadata
	if metadata == nil {
		return fmt.Errorf("KMS key %s has no metadata", keyARN)
	}

	var problems []string
	if !metadata.Enabled {
		problems = append(problems, "key is not enabled")
	}
	if metadata.DeletionDate != nil {
		problems = append(problems, fmt.Sprintf("key is scheduled for deletion at %s", metadata.DeletionDate))
	}
	if metadata.KeyUsage != kmsTypes.KeyUsageTypeEncryptDecrypt {
		problems = append(problems, fmt.Sprintf("key usage is %q, expected ENCRYPT_DECRYPT", metadata.KeyUsage))
	}

	policyResp, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
		KeyId:      &keyARN,
		PolicyName: stringPtr("default"),
	})
	if err != nil {
		return fmt.Errorf("getting KMS key policy: %w", err)
	}
	if policyResp.Policy == nil || !strings.Contains(*policyResp.Policy, operatorRolePrefix) {
		problems = append(problems, fmt.Sprintf("key policy does not reference the cluster operator roles (prefix %q)", operatorRolePrefix))
	}

	if len(problems) > 0 {
		return fmt.Errorf("KMS key %s is unhealthy:\n%s", keyARN, strings.Join(problems, "\n"))
	}
	return nil
}
