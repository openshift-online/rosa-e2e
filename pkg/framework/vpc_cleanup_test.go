package framework

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockEC2Client struct {
	describeNetworkInterfacesFn  func(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	detachNetworkInterfaceFn     func(ctx context.Context, params *ec2.DetachNetworkInterfaceInput, optFns ...func(*ec2.Options)) (*ec2.DetachNetworkInterfaceOutput, error)
	deleteNetworkInterfaceFn     func(ctx context.Context, params *ec2.DeleteNetworkInterfaceInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNetworkInterfaceOutput, error)
	describeSecurityGroupsFn     func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	revokeSecurityGroupIngressFn func(ctx context.Context, params *ec2.RevokeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error)
	revokeSecurityGroupEgressFn  func(ctx context.Context, params *ec2.RevokeSecurityGroupEgressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupEgressOutput, error)
	deleteSecurityGroupFn        func(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
	describeSubnetsFn            func(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	deleteSubnetFn               func(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error)
}

func (m *mockEC2Client) DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	if m.describeNetworkInterfacesFn != nil {
		return m.describeNetworkInterfacesFn(ctx, params, optFns...)
	}
	return &ec2.DescribeNetworkInterfacesOutput{}, nil
}

func (m *mockEC2Client) DetachNetworkInterface(ctx context.Context, params *ec2.DetachNetworkInterfaceInput, optFns ...func(*ec2.Options)) (*ec2.DetachNetworkInterfaceOutput, error) {
	if m.detachNetworkInterfaceFn != nil {
		return m.detachNetworkInterfaceFn(ctx, params, optFns...)
	}
	return &ec2.DetachNetworkInterfaceOutput{}, nil
}

func (m *mockEC2Client) DeleteNetworkInterface(ctx context.Context, params *ec2.DeleteNetworkInterfaceInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNetworkInterfaceOutput, error) {
	if m.deleteNetworkInterfaceFn != nil {
		return m.deleteNetworkInterfaceFn(ctx, params, optFns...)
	}
	return &ec2.DeleteNetworkInterfaceOutput{}, nil
}

func (m *mockEC2Client) DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if m.describeSecurityGroupsFn != nil {
		return m.describeSecurityGroupsFn(ctx, params, optFns...)
	}
	return &ec2.DescribeSecurityGroupsOutput{}, nil
}

func (m *mockEC2Client) RevokeSecurityGroupIngress(ctx context.Context, params *ec2.RevokeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	if m.revokeSecurityGroupIngressFn != nil {
		return m.revokeSecurityGroupIngressFn(ctx, params, optFns...)
	}
	return &ec2.RevokeSecurityGroupIngressOutput{}, nil
}

func (m *mockEC2Client) RevokeSecurityGroupEgress(ctx context.Context, params *ec2.RevokeSecurityGroupEgressInput, optFns ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupEgressOutput, error) {
	if m.revokeSecurityGroupEgressFn != nil {
		return m.revokeSecurityGroupEgressFn(ctx, params, optFns...)
	}
	return &ec2.RevokeSecurityGroupEgressOutput{}, nil
}

func (m *mockEC2Client) DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	if m.deleteSecurityGroupFn != nil {
		return m.deleteSecurityGroupFn(ctx, params, optFns...)
	}
	return &ec2.DeleteSecurityGroupOutput{}, nil
}

func (m *mockEC2Client) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if m.describeSubnetsFn != nil {
		return m.describeSubnetsFn(ctx, params, optFns...)
	}
	return &ec2.DescribeSubnetsOutput{}, nil
}

func (m *mockEC2Client) DeleteSubnet(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
	if m.deleteSubnetFn != nil {
		return m.deleteSubnetFn(ctx, params, optFns...)
	}
	return &ec2.DeleteSubnetOutput{}, nil
}

func speedUpTimers(t *testing.T) {
	origTimeout := eniDetachTimeout
	origPoll := eniDetachPollInterval
	origRetries := eniDeleteMaxRetries
	origDelay := eniDeleteRetryDelay

	eniDetachTimeout = 500 * time.Millisecond
	eniDetachPollInterval = 1 * time.Millisecond
	eniDeleteMaxRetries = 2
	eniDeleteRetryDelay = 1 * time.Millisecond

	t.Cleanup(func() {
		eniDetachTimeout = origTimeout
		eniDetachPollInterval = origPoll
		eniDeleteMaxRetries = origRetries
		eniDeleteRetryDelay = origDelay
	})
}

func TestCleanupVPCResources(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func() *mockEC2Client
		wantErr     bool
		errContains string
		logContains []string
	}{
		{
			name: "successful full cleanup",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeNetworkInterfacesFn: func(_ context.Context, params *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
						if len(params.Filters) > 0 {
							return &ec2.DescribeNetworkInterfacesOutput{
								NetworkInterfaces: []types.NetworkInterface{
									{
										NetworkInterfaceId: aws.String("eni-attached"),
										Status:             types.NetworkInterfaceStatusInUse,
										Attachment: &types.NetworkInterfaceAttachment{
											AttachmentId: aws.String("attach-123"),
										},
									},
									{
										NetworkInterfaceId: aws.String("eni-available"),
										Status:             types.NetworkInterfaceStatusAvailable,
									},
								},
							}, nil
						}
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{
								{
									NetworkInterfaceId: aws.String("eni-attached"),
									Status:             types.NetworkInterfaceStatusAvailable,
								},
							},
						}, nil
					},
					describeSecurityGroupsFn: func(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []types.SecurityGroup{
								{
									GroupId:   aws.String("sg-default"),
									GroupName: aws.String("default"),
								},
								{
									GroupId:   aws.String("sg-custom"),
									GroupName: aws.String("rosa-e2e-worker"),
									IpPermissions: []types.IpPermission{
										{IpProtocol: aws.String("-1")},
									},
								},
							},
						}, nil
					},
					describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []types.Subnet{
								{SubnetId: aws.String("subnet-1")},
								{SubnetId: aws.String("subnet-2")},
							},
						}, nil
					},
				}
			},
			logContains: []string{
				"Found 2 ENIs",
				"Detaching ENI eni-attached",
				"Deleted ENI eni-attached",
				"Deleted ENI eni-available",
				"Found 1 non-default security groups",
				"Deleted security group sg-custom",
				"Found 2 subnets",
				"Deleted subnet subnet-1",
				"Deleted subnet subnet-2",
				"cleanup complete",
			},
		},
		{
			name: "empty VPC no-op",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{}
			},
			logContains: []string{"cleanup complete"},
		},
		{
			name: "ENI detach failure",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeNetworkInterfacesFn: func(_ context.Context, params *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
						if len(params.Filters) > 0 {
							return &ec2.DescribeNetworkInterfacesOutput{
								NetworkInterfaces: []types.NetworkInterface{
									{
										NetworkInterfaceId: aws.String("eni-stuck"),
										Status:             types.NetworkInterfaceStatusInUse,
										Attachment: &types.NetworkInterfaceAttachment{
											AttachmentId: aws.String("attach-stuck"),
										},
									},
								},
							}, nil
						}
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
					detachNetworkInterfaceFn: func(_ context.Context, _ *ec2.DetachNetworkInterfaceInput, _ ...func(*ec2.Options)) (*ec2.DetachNetworkInterfaceOutput, error) {
						return nil, fmt.Errorf("OperationNotPermitted: cannot detach primary network interface")
					},
				}
			},
			wantErr:     true,
			errContains: "detaching ENI eni-stuck",
			logContains: []string{
				"Found 1 ENIs",
				"Detaching ENI eni-stuck",
			},
		},
		{
			name: "SG DependencyViolation",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeSecurityGroupsFn: func(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []types.SecurityGroup{
								{
									GroupId:   aws.String("sg-blocked"),
									GroupName: aws.String("rosa-worker-sg"),
								},
							},
						}, nil
					},
					deleteSecurityGroupFn: func(_ context.Context, _ *ec2.DeleteSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
						return nil, fmt.Errorf("DependencyViolation: resource sg-blocked has a dependent object")
					},
				}
			},
			wantErr:     true,
			errContains: "sg-blocked",
			logContains: []string{
				"Found 1 non-default security groups",
			},
		},
		{
			name: "partial subnet cleanup reporting",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: []types.Subnet{
								{SubnetId: aws.String("subnet-ok")},
								{SubnetId: aws.String("subnet-fail-1")},
								{SubnetId: aws.String("subnet-fail-2")},
							},
						}, nil
					},
					deleteSubnetFn: func(_ context.Context, params *ec2.DeleteSubnetInput, _ ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
						if aws.ToString(params.SubnetId) == "subnet-ok" {
							return &ec2.DeleteSubnetOutput{}, nil
						}
						return nil, fmt.Errorf("DependencyViolation: subnet %s has dependencies", aws.ToString(params.SubnetId))
					},
				}
			},
			wantErr:     true,
			errContains: "failed to delete 2 subnets",
			logContains: []string{
				"Found 3 subnets",
				"Deleted subnet subnet-ok",
			},
		},
		{
			name: "mixed ENI states",
			setupMock: func() *mockEC2Client {
				detachCalls := 0
				deleteCalls := 0
				return &mockEC2Client{
					describeNetworkInterfacesFn: func(_ context.Context, params *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
						if len(params.Filters) > 0 {
							return &ec2.DescribeNetworkInterfacesOutput{
								NetworkInterfaces: []types.NetworkInterface{
									{
										NetworkInterfaceId: aws.String("eni-1"),
										Status:             types.NetworkInterfaceStatusInUse,
										Attachment: &types.NetworkInterfaceAttachment{
											AttachmentId: aws.String("attach-1"),
										},
									},
									{
										NetworkInterfaceId: aws.String("eni-2"),
										Status:             types.NetworkInterfaceStatusAvailable,
									},
									{
										NetworkInterfaceId: aws.String("eni-3"),
										Status:             types.NetworkInterfaceStatusInUse,
										Attachment: &types.NetworkInterfaceAttachment{
											AttachmentId: aws.String("attach-3"),
										},
									},
								},
							}, nil
						}
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{
								{Status: types.NetworkInterfaceStatusAvailable},
							},
						}, nil
					},
					detachNetworkInterfaceFn: func(_ context.Context, _ *ec2.DetachNetworkInterfaceInput, _ ...func(*ec2.Options)) (*ec2.DetachNetworkInterfaceOutput, error) {
						detachCalls++
						return &ec2.DetachNetworkInterfaceOutput{}, nil
					},
					deleteNetworkInterfaceFn: func(_ context.Context, _ *ec2.DeleteNetworkInterfaceInput, _ ...func(*ec2.Options)) (*ec2.DeleteNetworkInterfaceOutput, error) {
						deleteCalls++
						return &ec2.DeleteNetworkInterfaceOutput{}, nil
					},
					describeSecurityGroupsFn: func(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						if detachCalls != 2 {
							t.Errorf("expected 2 detach calls (for eni-1 and eni-3), got %d", detachCalls)
						}
						if deleteCalls != 3 {
							t.Errorf("expected 3 delete calls (all ENIs), got %d", deleteCalls)
						}
						return &ec2.DescribeSecurityGroupsOutput{}, nil
					},
				}
			},
			logContains: []string{
				"Found 3 ENIs",
				"Detaching ENI eni-1",
				"Deleted ENI eni-2",
				"Detaching ENI eni-3",
			},
		},
		{
			name: "ENI delete retries on eventual consistency",
			setupMock: func() *mockEC2Client {
				deleteAttempts := 0
				return &mockEC2Client{
					describeNetworkInterfacesFn: func(_ context.Context, params *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
						if len(params.Filters) > 0 {
							return &ec2.DescribeNetworkInterfacesOutput{
								NetworkInterfaces: []types.NetworkInterface{
									{
										NetworkInterfaceId: aws.String("eni-flaky"),
										Status:             types.NetworkInterfaceStatusAvailable,
									},
								},
							}, nil
						}
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
					deleteNetworkInterfaceFn: func(_ context.Context, _ *ec2.DeleteNetworkInterfaceInput, _ ...func(*ec2.Options)) (*ec2.DeleteNetworkInterfaceOutput, error) {
						deleteAttempts++
						if deleteAttempts < 2 {
							return nil, fmt.Errorf("InvalidParameterValue: ENI is still in use")
						}
						return &ec2.DeleteNetworkInterfaceOutput{}, nil
					},
				}
			},
			logContains: []string{"Deleted ENI eni-flaky"},
		},
		{
			name: "ENI already gone during wait",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeNetworkInterfacesFn: func(_ context.Context, params *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
						if len(params.Filters) > 0 {
							return &ec2.DescribeNetworkInterfacesOutput{
								NetworkInterfaces: []types.NetworkInterface{
									{
										NetworkInterfaceId: aws.String("eni-vanish"),
										Status:             types.NetworkInterfaceStatusInUse,
										Attachment: &types.NetworkInterfaceAttachment{
											AttachmentId: aws.String("attach-vanish"),
										},
									},
								},
							}, nil
						}
						return nil, fmt.Errorf("InvalidNetworkInterfaceID.NotFound: eni-vanish does not exist")
					},
					deleteNetworkInterfaceFn: func(_ context.Context, _ *ec2.DeleteNetworkInterfaceInput, _ ...func(*ec2.Options)) (*ec2.DeleteNetworkInterfaceOutput, error) {
						return nil, fmt.Errorf("InvalidNetworkInterfaceID.NotFound: eni-vanish does not exist")
					},
				}
			},
			logContains: []string{"ENI eni-vanish already gone"},
		},
		{
			name: "default security group is skipped",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeSecurityGroupsFn: func(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []types.SecurityGroup{
								{
									GroupId:   aws.String("sg-default-1"),
									GroupName: aws.String("default"),
								},
							},
						}, nil
					},
					deleteSecurityGroupFn: func(_ context.Context, _ *ec2.DeleteSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
						t.Error("DeleteSecurityGroup should not be called for default SG")
						return nil, nil
					},
				}
			},
			logContains: []string{"cleanup complete"},
		},
		{
			name: "SG with cross-references revoked before delete",
			setupMock: func() *mockEC2Client {
				ingressRevoked := false
				egressRevoked := false
				return &mockEC2Client{
					describeSecurityGroupsFn: func(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []types.SecurityGroup{
								{
									GroupId:   aws.String("sg-a"),
									GroupName: aws.String("sg-a-name"),
									IpPermissions: []types.IpPermission{
										{
											IpProtocol: aws.String("-1"),
											UserIdGroupPairs: []types.UserIdGroupPair{
												{GroupId: aws.String("sg-b")},
											},
										},
									},
									IpPermissionsEgress: []types.IpPermission{
										{
											IpProtocol: aws.String("-1"),
											UserIdGroupPairs: []types.UserIdGroupPair{
												{GroupId: aws.String("sg-b")},
											},
										},
									},
								},
								{
									GroupId:   aws.String("sg-b"),
									GroupName: aws.String("sg-b-name"),
									IpPermissions: []types.IpPermission{
										{
											IpProtocol: aws.String("-1"),
											UserIdGroupPairs: []types.UserIdGroupPair{
												{GroupId: aws.String("sg-a")},
											},
										},
									},
								},
							},
						}, nil
					},
					revokeSecurityGroupIngressFn: func(_ context.Context, _ *ec2.RevokeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
						ingressRevoked = true
						return &ec2.RevokeSecurityGroupIngressOutput{}, nil
					},
					revokeSecurityGroupEgressFn: func(_ context.Context, _ *ec2.RevokeSecurityGroupEgressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupEgressOutput, error) {
						egressRevoked = true
						return &ec2.RevokeSecurityGroupEgressOutput{}, nil
					},
					deleteSecurityGroupFn: func(_ context.Context, _ *ec2.DeleteSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
						if !ingressRevoked || !egressRevoked {
							return nil, fmt.Errorf("DependencyViolation: rules not revoked yet")
						}
						return &ec2.DeleteSecurityGroupOutput{}, nil
					},
				}
			},
			logContains: []string{
				"Deleted security group sg-a",
				"Deleted security group sg-b",
			},
		},
		{
			name: "paginated ENI, SG, and subnet listing",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeNetworkInterfacesFn: func(_ context.Context, params *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
						if len(params.Filters) > 0 {
							if params.NextToken == nil {
								return &ec2.DescribeNetworkInterfacesOutput{
									NetworkInterfaces: []types.NetworkInterface{
										{NetworkInterfaceId: aws.String("eni-page1"), Status: types.NetworkInterfaceStatusAvailable},
									},
									NextToken: aws.String("page2"),
								}, nil
							}
							return &ec2.DescribeNetworkInterfacesOutput{
								NetworkInterfaces: []types.NetworkInterface{
									{NetworkInterfaceId: aws.String("eni-page2"), Status: types.NetworkInterfaceStatusAvailable},
								},
							}, nil
						}
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
					describeSecurityGroupsFn: func(_ context.Context, params *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						if params.NextToken == nil {
							return &ec2.DescribeSecurityGroupsOutput{
								SecurityGroups: []types.SecurityGroup{
									{GroupId: aws.String("sg-default"), GroupName: aws.String("default")},
									{GroupId: aws.String("sg-p1"), GroupName: aws.String("sg-page1")},
								},
								NextToken: aws.String("page2"),
							}, nil
						}
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []types.SecurityGroup{
								{GroupId: aws.String("sg-p2"), GroupName: aws.String("sg-page2")},
							},
						}, nil
					},
					describeSubnetsFn: func(_ context.Context, params *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
						if params.NextToken == nil {
							return &ec2.DescribeSubnetsOutput{
								Subnets: []types.Subnet{
									{SubnetId: aws.String("subnet-p1")},
								},
								NextToken: aws.String("page2"),
							}, nil
						}
						return &ec2.DescribeSubnetsOutput{
							Subnets: []types.Subnet{
								{SubnetId: aws.String("subnet-p2")},
							},
						}, nil
					},
				}
			},
			logContains: []string{
				"Found 2 ENIs",
				"Deleted ENI eni-page1",
				"Deleted ENI eni-page2",
				"Found 2 non-default security groups",
				"Deleted security group sg-p1",
				"Deleted security group sg-p2",
				"Found 2 subnets",
				"Deleted subnet subnet-p1",
				"Deleted subnet subnet-p2",
				"cleanup complete",
			},
		},
		{
			name: "SG revoke failure continues to delete pass",
			setupMock: func() *mockEC2Client {
				return &mockEC2Client{
					describeSecurityGroupsFn: func(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
						return &ec2.DescribeSecurityGroupsOutput{
							SecurityGroups: []types.SecurityGroup{
								{
									GroupId:   aws.String("sg-fail-revoke"),
									GroupName: aws.String("fail-revoke"),
									IpPermissions: []types.IpPermission{
										{IpProtocol: aws.String("-1")},
									},
								},
								{
									GroupId:   aws.String("sg-ok"),
									GroupName: aws.String("ok-sg"),
								},
							},
						}, nil
					},
					revokeSecurityGroupIngressFn: func(_ context.Context, _ *ec2.RevokeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
						return nil, fmt.Errorf("access denied")
					},
					deleteSecurityGroupFn: func(_ context.Context, params *ec2.DeleteSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
						if aws.ToString(params.GroupId) == "sg-fail-revoke" {
							return nil, fmt.Errorf("DependencyViolation: rules not revoked")
						}
						return &ec2.DeleteSecurityGroupOutput{}, nil
					},
				}
			},
			wantErr:     true,
			errContains: "sg-fail-revoke",
			logContains: []string{
				"Found 2 non-default security groups",
				"Deleted security group sg-ok",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			speedUpTimers(t)

			mock := tt.setupMock()
			var logBuf bytes.Buffer

			err := CleanupVPCResources(context.Background(), mock, "vpc-test", &logBuf)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			logOutput := logBuf.String()
			for _, want := range tt.logContains {
				if !strings.Contains(logOutput, want) {
					t.Errorf("log output missing %q\nfull log:\n%s", want, logOutput)
				}
			}
		})
	}
}
