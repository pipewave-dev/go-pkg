package exobjstore

// import (
// 	"bytes"
// 	"context"
// 	"sync"

// 	"github.com/pipewave-dev/go-pkg/global"
// 	"github.com/pipewave-dev/go-pkg/pkg/s3"
// )

// var (
// 	once     sync.Once
// 	s3forLog s3.S3Provider
// )

// func initS3ProviderForSlog() {
// 	once.Do(func() {
// 		s3Cfn := s3.S3Config{
// 			Endpoint:        config.GetEnv().HttpClientLog.S3.Endpoint,
// 			Region:          config.GetEnv().HttpClientLog.S3.Region,
// 			Bucket:          config.GetEnv().HttpClientLog.S3.Bucket,
// 			Role:            config.GetEnv().HttpClientLog.S3.Role,
// 			StaticAccessKey: config.GetEnv().HttpClientLog.S3.StaticAccessKey,
// 			StaticSecretKey: config.GetEnv().HttpClientLog.S3.StaticSecretKey,
// 		}
// 		s3forLog = s3.NewS3Provider(&s3Cfn)
// 	})
// }

// /*
// # UploadFn

// Upload any content to s3 and return S3 key path.
// This function will run in background and return immediately.
// This function useful when using for log very long content.
// */
// func UploadFn(uploadedS3Key string, content []byte) error {
// 	initS3ProviderForSlog()

// 	_, _, _, err := s3forLog.UploadContentT2(
// 		context.Background(),
// 		bytes.NewReader(content),
// 		&uploadedS3Key,
// 	)

// 	return err
// }
