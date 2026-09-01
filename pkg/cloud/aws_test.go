package cloud

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
)

type MockSTSClient struct {
	CapturedRoleArn    string
	CapturedExternalID string
}

func (m *MockSTSClient) AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	if params.RoleArn != nil {
		m.CapturedRoleArn = *params.RoleArn
	}
	if params.ExternalId != nil {
		m.CapturedExternalID = *params.ExternalId
	}

	return &sts.AssumeRoleOutput{
		Credentials: &types.Credentials{
			AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
			SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			SessionToken:    aws.String("AQoDYXdzEJr1KEXAMPLE"),
		},
	}, nil
}

func TestAssumeRoleWithExternalID(t *testing.T) {
	mockSTS := &MockSTSClient{}
	scanner := NewCloudScanner(aws.Config{Region: "us-east-1"}, mockSTS)

	roleArn := "arn:aws:iam::123456789012:role/TertiusEyeCrossAccountRole"
	externalID := "tenant-external-id-secret-999"

	inv, err := scanner.AssumeRoleAndScan(context.Background(), roleArn, externalID, "UnitTestSession")
	if err != nil {
		t.Fatalf("AssumeRoleAndScan failed: %v", err)
	}

	if inv == nil {
		t.Fatal("Expected non-nil cloud inventory")
	}

	if mockSTS.CapturedRoleArn != roleArn {
		t.Errorf("Expected RoleArn '%s', got '%s'", roleArn, mockSTS.CapturedRoleArn)
	}

	if mockSTS.CapturedExternalID != externalID {
		t.Errorf("Expected ExternalID '%s', got '%s'", externalID, mockSTS.CapturedExternalID)
	}
}
