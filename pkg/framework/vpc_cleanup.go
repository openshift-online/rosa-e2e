package framework

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// EC2VPCClient defines the EC2 API subset needed for VPC resource cleanup.
// *ec2.Client satisfies this interface.
type EC2VPCClient interface {
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	DetachNetworkInterface(ctx context.Context, params *ec2.DetachNetworkInterfaceInput, optFns ...func(*ec2.Options)) (*ec2.DetachNetworkInterfaceOutput, error)
	DeleteNetworkInterface(ctx context.Context, params *ec2.DeleteNetworkInterfaceInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNetworkInterfaceOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	RevokeSecurityGroupIngress(ctx context.Context, params *ec2.RevokeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupEgress(ctx context.Context, params *ec2.RevokeSecurityGroupEgressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupEgressOutput, error)
	DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DeleteSubnet(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error)
}

// Timing configuration; package-level vars so tests can override for speed.
var (
	eniDetachTimeout      = 2 * time.Minute
	eniDetachPollInterval = 5 * time.Second
	eniDeleteMaxRetries   = 5
	eniDeleteRetryDelay   = 3 * time.Second
)

// CleanupVPCResources cascade-deletes dependent AWS resources in a VPC so it
// can be reused or recreated cleanly. Deletion order: ENIs → security groups → subnets.
// Returns a hard error identifying blocking resources if any step fails.
func CleanupVPCResources(ctx context.Context, client EC2VPCClient, vpcID string, log io.Writer) error {
	if err := cleanupENIs(ctx, client, vpcID, log); err != nil {
		return fmt.Errorf("cleaning up ENIs in VPC %s: %w", vpcID, err)
	}

	if err := cleanupSecurityGroups(ctx, client, vpcID, log); err != nil {
		return fmt.Errorf("cleaning up security groups in VPC %s: %w", vpcID, err)
	}

	if err := cleanupSubnets(ctx, client, vpcID, log); err != nil {
		return fmt.Errorf("cleaning up subnets in VPC %s: %w", vpcID, err)
	}

	fmt.Fprintf(log, "VPC %s resource cleanup complete\n", vpcID)
	return nil
}

func cleanupENIs(ctx context.Context, client EC2VPCClient, vpcID string, log io.Writer) error {
	resp, err := client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil {
		return fmt.Errorf("describing network interfaces: %w", err)
	}

	if len(resp.NetworkInterfaces) == 0 {
		return nil
	}

	fmt.Fprintf(log, "Found %d ENIs in VPC %s\n", len(resp.NetworkInterfaces), vpcID)

	for _, eni := range resp.NetworkInterfaces {
		eniID := aws.ToString(eni.NetworkInterfaceId)

		if eni.Attachment != nil && eni.Attachment.AttachmentId != nil {
			fmt.Fprintf(log, "Detaching ENI %s (attachment: %s)\n", eniID, aws.ToString(eni.Attachment.AttachmentId))
			_, err := client.DetachNetworkInterface(ctx, &ec2.DetachNetworkInterfaceInput{
				AttachmentId: eni.Attachment.AttachmentId,
				Force:        aws.Bool(true),
			})
			if err != nil {
				return fmt.Errorf("detaching ENI %s (attachment %s): %w",
					eniID, aws.ToString(eni.Attachment.AttachmentId), err)
			}

			if err := waitForENIAvailable(ctx, client, eniID); err != nil {
				return fmt.Errorf("waiting for ENI %s to detach: %w", eniID, err)
			}
		}

		if err := deleteENIWithRetry(ctx, client, eniID); err != nil {
			return fmt.Errorf("deleting ENI %s: %w", eniID, err)
		}

		fmt.Fprintf(log, "Deleted ENI %s\n", eniID)
	}

	return nil
}

func waitForENIAvailable(ctx context.Context, client EC2VPCClient, eniID string) error {
	deadline := time.Now().Add(eniDetachTimeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for ENI %s to reach available state", eniDetachTimeout, eniID)
		}

		resp, err := client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []string{eniID},
		})
		if err != nil {
			// ENI was already deleted
			if strings.Contains(err.Error(), "InvalidNetworkInterfaceID") {
				return nil
			}
			return fmt.Errorf("describing ENI %s: %w", eniID, err)
		}

		if len(resp.NetworkInterfaces) == 0 {
			return nil
		}

		if resp.NetworkInterfaces[0].Status == types.NetworkInterfaceStatusAvailable {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(eniDetachPollInterval):
		}
	}
}

func deleteENIWithRetry(ctx context.Context, client EC2VPCClient, eniID string) error {
	var lastErr error
	for attempt := range eniDeleteMaxRetries {
		_, err := client.DeleteNetworkInterface(ctx, &ec2.DeleteNetworkInterfaceInput{
			NetworkInterfaceId: aws.String(eniID),
		})
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "InvalidNetworkInterfaceID") {
			return nil
		}
		lastErr = err

		if attempt < eniDeleteMaxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(eniDeleteRetryDelay):
			}
		}
	}
	return fmt.Errorf("after %d attempts: %w", eniDeleteMaxRetries, lastErr)
}

func cleanupSecurityGroups(ctx context.Context, client EC2VPCClient, vpcID string, log io.Writer) error {
	resp, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil {
		return fmt.Errorf("describing security groups: %w", err)
	}

	var sgs []types.SecurityGroup
	for _, sg := range resp.SecurityGroups {
		if aws.ToString(sg.GroupName) != "default" {
			sgs = append(sgs, sg)
		}
	}

	if len(sgs) == 0 {
		return nil
	}

	fmt.Fprintf(log, "Found %d non-default security groups in VPC %s\n", len(sgs), vpcID)

	// Revoke all rules first to break cross-SG reference cycles
	for _, sg := range sgs {
		sgID := aws.ToString(sg.GroupId)

		if len(sg.IpPermissions) > 0 {
			_, err := client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
				GroupId:       sg.GroupId,
				IpPermissions: sg.IpPermissions,
			})
			if err != nil {
				return fmt.Errorf("revoking ingress rules for SG %s: %w", sgID, err)
			}
		}

		if len(sg.IpPermissionsEgress) > 0 {
			_, err := client.RevokeSecurityGroupEgress(ctx, &ec2.RevokeSecurityGroupEgressInput{
				GroupId:       sg.GroupId,
				IpPermissions: sg.IpPermissionsEgress,
			})
			if err != nil {
				return fmt.Errorf("revoking egress rules for SG %s: %w", sgID, err)
			}
		}
	}

	var deleteErrors []string
	for _, sg := range sgs {
		sgID := aws.ToString(sg.GroupId)
		sgName := aws.ToString(sg.GroupName)
		_, err := client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: sg.GroupId,
		})
		if err != nil {
			deleteErrors = append(deleteErrors, fmt.Sprintf("SG %s (%s): %v", sgID, sgName, err))
			continue
		}
		fmt.Fprintf(log, "Deleted security group %s (%s)\n", sgID, sgName)
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("failed to delete %d security groups:\n%s", len(deleteErrors), strings.Join(deleteErrors, "\n"))
	}

	return nil
}

func cleanupSubnets(ctx context.Context, client EC2VPCClient, vpcID string, log io.Writer) error {
	resp, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil {
		return fmt.Errorf("describing subnets: %w", err)
	}

	if len(resp.Subnets) == 0 {
		return nil
	}

	fmt.Fprintf(log, "Found %d subnets in VPC %s\n", len(resp.Subnets), vpcID)

	var deleteErrors []string
	for _, subnet := range resp.Subnets {
		subnetID := aws.ToString(subnet.SubnetId)
		_, err := client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{
			SubnetId: subnet.SubnetId,
		})
		if err != nil {
			deleteErrors = append(deleteErrors, fmt.Sprintf("subnet %s: %v", subnetID, err))
			continue
		}
		fmt.Fprintf(log, "Deleted subnet %s\n", subnetID)
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("failed to delete %d subnets:\n%s", len(deleteErrors), strings.Join(deleteErrors, "\n"))
	}

	return nil
}
