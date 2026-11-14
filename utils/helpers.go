package utils

import (
	"bytes"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"golang.org/x/crypto/bcrypt"
)

const RandomCharPool = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const SecretHashCost = 12

func HashSecret(secret string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(secret), SecretHashCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func VerifySecret(stored, candidate string) bool {
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(candidate)) == nil
}

func RandomString(length uint) string {
	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = RandomCharPool[rand.Intn(len(RandomCharPool))]
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
	fileBytes, err := io.ReadAll(file)
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
