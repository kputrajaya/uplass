package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const randomCharPool = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func HashString(input string) string {
	hashed := sha256.Sum256([]byte(input))
	return base64.StdEncoding.EncodeToString(hashed[:])
}

func RandomString(length uint) string {
	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = randomCharPool[rand.Intn(len(randomCharPool))]
	}
	return string(bytes)
}

func UploadToS3(fileHeader *multipart.FileHeader, prefix string) error {
	awsBucket := os.Getenv("AWS_BUCKET")
	awsRegion := os.Getenv("AWS_REGION")
	awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
	awsSecretKey := os.Getenv("AWS_SECRET_KEY")
	if awsBucket == "" || awsRegion == "" || awsAccessKey == "" || awsSecretKey == "" {
		log.Fatal("Invalid AWS settings")
	}

	// Create AWS session
	session, err := session.NewSession(&aws.Config{
		Region:      aws.String(awsRegion),
		Credentials: credentials.NewStaticCredentials(awsAccessKey, awsSecretKey, ""),
	})
	if err != nil {
		return err
	}

	// Open file from header
	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	// Put file to S3
	_, err = s3.New(session).PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(awsBucket),
		Key:           aws.String(prefix + fileHeader.Filename),
		Body:          bytes.NewReader(fileBytes),
		ContentLength: aws.Int64(fileHeader.Size),
		ContentType:   aws.String(http.DetectContentType(fileBytes)),
	})
	return err
}
