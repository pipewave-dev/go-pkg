package awsutils

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

/*
CreateCredentials .. Create custom aws.Config easier than using a lot of stuff config from document.
Warning: panic occur if there's any error
https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/endpoints/

	example:

		import "github.com/aws/aws-sdk-go-v2/service/s3"

		cfg := CreateCredentials("ap-northeast-1", "", "", "arn:aws:iam::123456789012:role/S3ReadRole")
		s3Client := s3.NewFromConfig(cfg)
*/
func CreateCredentials(region, profile, staticAccessKey, staticSecretKey, assumeRole string) aws.Config {

	awsCfnOptFns := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	// Custom retryer with exponential backoff
	customRetryer := retry.NewStandard(func(so *retry.StandardOptions) {
		so.MaxAttempts = 4               // Maximum 4 attempts
		so.MaxBackoff = time.Second * 30 // Max backoff delay
		so.Backoff = retry.NewExponentialJitterBackoff(
			time.Second * 30, // Max delay
		)
	})
	awsCfnOptFns = append(
		awsCfnOptFns,
		config.WithRetryer(func() aws.Retryer {
			return customRetryer
		}),
	)

	// Profile
	if profile != "" {
		awsCfnOptFns = append(awsCfnOptFns,
			config.WithSharedConfigProfile(profile))
	}

	// Static config (only for local development)
	if staticAccessKey != "" && staticSecretKey != "" {
		slog.Warn("don't use static credentials in production")
		awsCfnOptFns = append(
			awsCfnOptFns,
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				staticAccessKey,
				staticSecretKey,
				"")),
		)
	}

	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		awsCfnOptFns...,
	)
	if err != nil {
		panic(err)
	}

	if assumeRole != "" {
		// Create the credentials from AssumeRoleProvider to assume the role
		// referenced by the "myRoleARN" ARN.
		stsSvc := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsSvc, assumeRole)
		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return cfg
}
