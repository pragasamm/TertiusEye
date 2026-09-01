package cloud

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// STSClientAPI interface for AWS STS client (allows mocking in tests).
type STSClientAPI interface {
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

// CloudScanner manages AWS cross-account resource discovery using STS AssumeRole with ExternalID protection.
type CloudScanner struct {
	stsClient STSClientAPI
	baseCfg   aws.Config
}

// CloudInventory holds discovered cloud resources.
type CloudInventory struct {
	EC2Instances []string `json:"ec2_instances"`
	S3Buckets    []string `json:"s3_buckets"`
}

// NewCloudScanner initializes a CloudScanner instance.
func NewCloudScanner(baseCfg aws.Config, stsClient STSClientAPI) *CloudScanner {
	if stsClient == nil {
		stsClient = sts.NewFromConfig(baseCfg)
	}
	return &CloudScanner{
		stsClient: stsClient,
		baseCfg:   baseCfg,
	}
}

// AssumeRoleAndScan retrieves temporary credentials via sts.AssumeRole with ExternalId validation (LLD 4.3)
// to prevent the Confused Deputy vulnerability, then scans EC2 and S3 resources.
func (c *CloudScanner) AssumeRoleAndScan(ctx context.Context, roleArn, externalID, sessionName string) (*CloudInventory, error) {
	if roleArn == "" || externalID == "" {
		return nil, fmt.Errorf("roleArn and externalID are required for cross-account assume role")
	}

	if sessionName == "" {
		sessionName = "TertiusEyeCloudDiscoverySession"
	}

	// LLD Section 4.3: Strictly pass ExternalId to mitigate Confused Deputy vulnerability
	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		ExternalId:      aws.String(externalID),
		RoleSessionName: aws.String(sessionName),
	}

	assumeRoleOutput, err := c.stsClient.AssumeRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("sts.AssumeRole failed for RoleArn %s with ExternalId: %w", roleArn, err)
	}

	if assumeRoleOutput.Credentials == nil {
		return nil, fmt.Errorf("sts.AssumeRole returned nil temporary credentials")
	}

	// Create temporary AWS credentials provider from AssumeRole output
	accessKeyID := ""
	if assumeRoleOutput.Credentials.AccessKeyId != nil {
		accessKeyID = *assumeRoleOutput.Credentials.AccessKeyId
	}
	secretAccessKey := ""
	if assumeRoleOutput.Credentials.SecretAccessKey != nil {
		secretAccessKey = *assumeRoleOutput.Credentials.SecretAccessKey
	}
	sessionToken := ""
	if assumeRoleOutput.Credentials.SessionToken != nil {
		sessionToken = *assumeRoleOutput.Credentials.SessionToken
	}

	tempCfg := c.baseCfg.Copy()
	tempCfg.Credentials = credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken)

	// Instantiate new AWS service clients using temporary assumed-role credentials
	ec2Client := ec2.NewFromConfig(tempCfg)
	s3Client := s3.NewFromConfig(tempCfg)

	inventory := &CloudInventory{}

	// Execute EC2 instances discovery
	ec2Resp, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
	if err == nil {
		for _, r := range ec2Resp.Reservations {
			for _, inst := range r.Instances {
				if inst.InstanceId != nil {
					inventory.EC2Instances = append(inventory.EC2Instances, *inst.InstanceId)
				}
			}
		}
	}

	// Execute S3 buckets discovery
	s3Resp, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err == nil {
		for _, b := range s3Resp.Buckets {
			if b.Name != nil {
				inventory.S3Buckets = append(inventory.S3Buckets, *b.Name)
			}
		}
	}

	return inventory, nil
}
