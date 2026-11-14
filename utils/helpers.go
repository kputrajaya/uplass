package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/crypto/bcrypt"
)

const secretHashCost = 12

func HashSecret(secret string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(secret), secretHashCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func VerifySecret(stored, candidate string) bool {
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(candidate)) == nil
}

func RandomString(length uint) string {
	var out strings.Builder
	for uint(out.Len()) < length {
		out.WriteString(rand.Text())
	}
	return out.String()[:length]
}

func UploadToS3(fileHeader *multipart.FileHeader, prefix string) error {
	awsBucket := os.Getenv("AWS_BUCKET")
	awsRegion := os.Getenv("AWS_REGION")
	awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
	awsSecretKey := os.Getenv("AWS_SECRET_KEY")
	if awsBucket == "" || awsRegion == "" || awsAccessKey == "" || awsSecretKey == "" {
		log.Fatal("Invalid AWS settings")
	}

	// Open file from header
	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Put file to S3
	client := s3.NewFromConfig(aws.Config{
		Region:      awsRegion,
		Credentials: credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, ""),
	})
	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(awsBucket),
		Key:         aws.String(prefix + fileHeader.Filename),
		Body:        bytes.NewReader(fileBytes),
		ContentType: aws.String(http.DetectContentType(fileBytes)),
	})
	return err
}
