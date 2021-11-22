package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"sample/s3"
	"sample/zip"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	tempArtifactPath = "/tmp/artifact/"
	tempZipPath      = tempArtifactPath + "zipped/"
	tempUnzipPath    = tempArtifactPath + "unzipped/"
	tempZip          = "temp.zip"
	dirPerm          = 0777
	region           = "ap-northeast-1"
)

var (
	now string
	// zipファイルをダウンロードするLmanbda上のパス
	zipContentPath string
	// zipファイルを解凍するlambda上のpath
	unzipContentPath string
	// 解凍したファイルをアップロードするs3上のバケット
	destBucket string
)

func init() {
	destBucket = os.Getenv("UNZIPPED_ARTIFACT_BUCKET")
}

func main() {
	lambda.Start(handler)
}

// func (context.Context, TIn) error を利用しました
// コンテキストからリクエストIDを取得して、s3アップロード時のイベントを利用するためです
func handler(ctx context.Context, s3Event events.S3Event) error {
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		log.Printf("AwsRequestID: %s", lc.AwsRequestID)
	}

	// s3Eventからはバケット名などが取得できます
	// 詳細はソースコードの構造体定義をみましょう
	// https://github.com/aws/aws-lambda-go/blob/master/events/s3.go
	bucket := s3Event.Records[0].S3.Bucket.Name
	key := s3Event.Records[0].S3.Object.Key

	log.Printf("bucket: %s , key: %s", bucket, key)

	if err := prepareDirectory(); err != nil {
		log.Fatal(err)
	}

	// AWSのサービス接続に必要な認証情報を初期化する
	// クレデンシャルをaws.Config経由で明示的に指定しない場合は
	// ~/.aws/credentials　が利用されます
	// 詳細はソースコードのコードみましょう
	// https://github.com/aws/aws-sdk-go/blob/master/aws/session/session.go#L97
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(region)}),
	)

	downloader := s3.NewDownloader(sess, bucket, key, zipContentPath+tempZip)
	downloadedZipPath, err := downloader.Download()
	if err != nil {
		log.Fatal(err)
	}

	if err := zip.Unzip(downloadedZipPath, unzipContentPath); err != nil {
		log.Fatal(err)
	}

	uploader := s3.NewUploader(sess, tempUnzipPath, destBucket)

	if err := uploader.Upload(); err != nil {
		log.Fatal(err)
	}

	log.Printf("%s unzipped to S3 bucket: %s", downloadedZipPath, destBucket)

	return nil
}

// Lambdaの実行環境では/tmpディレクトリに対する書き込みが可能です
// ただし512MBの制限がある
// また、実行環境（コンテナ）はリクエストの頻度により
// 再利用されることもあれば新規に作られることもあるので
// tmp配下のファイルの存在を前提としない実装にすることが大事です

func prepareDirectory() error {
	now = strconv.Itoa(int(time.Now().UnixNano()))
	zipContentPath = tempZipPath + now + "/"
	unzipContentPath = tempUnzipPath + now + "/"

	if _, err := os.Stat(tempArtifactPath); err == nil {
		if err := os.RemoveAll(tempArtifactPath); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(zipContentPath, dirPerm); err != nil {
		return err
	}
	if err := os.MkdirAll(unzipContentPath, dirPerm); err != nil {
		return err
	}

	return nil
}
