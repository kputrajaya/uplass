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
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/crypto/bcrypt"
)

// HashSecret is bcrypt, salt embedded in the hash
func HashSecret(secret string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// VerifySecret accepts only bcrypt hashes; anything else returns false
func VerifySecret(stored, candidate string) bool {
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(candidate)) == nil
}

// RandomString concatenates crypto/rand.Text() chunks: unbiased base32, may split a chunk
func RandomString(length uint) string {
	var out strings.Builder
	for uint(out.Len()) < length {
		out.WriteString(rand.Text())
	}
	return out.String()[:length]
}

// SafeFilename keeps the last path component and drops control characters
func SafeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)

	switch name {
	case "", ".", "..", "/":
		return "file"
	}
	return name
}

// UploadToS3 returns the stored path; build URLs from it, not from the request
func UploadToS3(fileHeader *multipart.FileHeader, prefix string) (string, error) {
	client, bucket := s3Target()
	filePath := prefix + SafeFilename(fileHeader.Filename)

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(filePath),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(http.DetectContentType(data)),
	})
	if err != nil {
		return "", err
	}
	return filePath, nil
}

func s3Target() (*s3.Client, string) {
	bucket := os.Getenv("AWS_BUCKET")
	region := os.Getenv("AWS_REGION")
	accessKey := os.Getenv("AWS_ACCESS_KEY")
	secretKey := os.Getenv("AWS_SECRET_KEY")
	if bucket == "" || region == "" || accessKey == "" || secretKey == "" {
		log.Fatal("Invalid AWS settings")
	}

	client := s3.NewFromConfig(aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}, func(o *s3.Options) {
		if endpoint := os.Getenv("AWS_ENDPOINT"); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})

	return client, bucket
}
