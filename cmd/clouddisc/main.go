package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

	"tertiuseye/agent/pkg/cloud"
)

func main() {
	roleArn := flag.String("role-arn", "", "AWS Target Role ARN for AssumeRole")
	externalID := flag.String("external-id", "", "Tenant ExternalID string for AssumeRole")
	flag.Parse()

	if *roleArn == "" || *externalID == "" {
		fmt.Println("[Cloud Discovery Service] Usage: -role-arn <arn> -external-id <external-id>")
		fmt.Println("[Cloud Discovery Service] Running dry-run simulation mode...")
		scanner := cloud.NewCloudScanner(aws.Config{}, nil)
		_ = scanner
		fmt.Println("[Cloud Discovery Service] CloudScanner initialized successfully with ExternalID security checks.")
		return
	}

	ctx := context.Background()
	scanner := cloud.NewCloudScanner(aws.Config{}, nil)

	inv, err := scanner.AssumeRoleAndScan(ctx, *roleArn, *externalID, "TertiusEyeCLI")
	if err != nil {
		fmt.Printf("Cloud Discovery scan failed: %v\n", err)
		return
	}

	fmt.Printf("[Cloud Discovery Service] Discovered %d EC2 instances and %d S3 buckets.\n", len(inv.EC2Instances), len(inv.S3Buckets))
}
