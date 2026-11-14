package utils

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHashSecret(t *testing.T) {
	hashed, err := HashSecret("s3cret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if hashed == "s3cret" {
		t.Fatal("HashSecret returned the plain secret")
	}
	if !strings.HasPrefix(hashed, "$2") {
		t.Fatalf("HashSecret = %q, want a bcrypt hash", hashed)
	}
	if !strings.HasPrefix(hashed, "$2a$12$") {
		t.Fatalf("HashSecret = %q, want cost 12", hashed)
	}

	second, err := HashSecret("s3cret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if second == hashed {
		t.Fatal("HashSecret is not salted: two hashes of the same secret matched")
	}
}

func TestVerifySecret(t *testing.T) {
	hashed, err := HashSecret("s3cret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}

	if !VerifySecret(hashed, "s3cret") {
		t.Fatal("VerifySecret rejected the correct secret")
	}
	if VerifySecret(hashed, "wrong") {
		t.Fatal("VerifySecret accepted a wrong secret")
	}
	if VerifySecret(hashed, "") {
		t.Fatal("VerifySecret accepted an empty secret")
	}
	if VerifySecret(hashed, "S3cret") {
		t.Fatal("VerifySecret is case insensitive")
	}
	if VerifySecret("", "s3cret") {
		t.Fatal("VerifySecret accepted an empty stored hash")
	}
	if VerifySecret("not-a-bcrypt-hash", "s3cret") {
		t.Fatal("VerifySecret accepted a non-bcrypt stored hash")
	}
}

const base32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

func TestRandomString(t *testing.T) {
	t.Run("length and charset", func(t *testing.T) {
		got := RandomString(50)
		if len(got) != 50 {
			t.Fatalf("len(RandomString(50)) = %d, want 50", len(got))
		}
		for _, r := range got {
			if !strings.ContainsRune(base32Alphabet, r) {
				t.Fatalf("RandomString produced character %q outside the base32 alphabet", r)
			}
		}
	})

	t.Run("zero length", func(t *testing.T) {
		if got := RandomString(0); got != "" {
			t.Fatalf("RandomString(0) = %q, want empty", got)
		}
	})

	t.Run("successive calls differ", func(t *testing.T) {
		first := RandomString(50)
		second := RandomString(50)
		if first == second {
			t.Fatalf("two RandomString calls returned the same value: %q", first)
		}
	})

	t.Run("values are unique", func(t *testing.T) {
		seen := make(map[string]struct{}, 1000)
		for i := 0; i < 1000; i++ {
			s := RandomString(50)
			if _, dup := seen[s]; dup {
				t.Fatalf("duplicate value on iteration %d: %q", i, s)
			}
			seen[s] = struct{}{}
		}
	})

	t.Run("exact Text boundary", func(t *testing.T) {
		for _, n := range []uint{1, 5, 25, 26, 27, 51, 52} {
			if got := RandomString(n); uint(len(got)) != n {
				t.Fatalf("len(RandomString(%d)) = %d, want %d", n, len(got), n)
			}
		}
	})
}

func fileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	_, header, err := req.FormFile("file")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	return header
}

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"pic.png":               "pic.png",
		"../evil.png":           "evil.png",
		"../../etc/passwd":      "passwd",
		`C:\Users\HomePC\a.txt`: "a.txt",
		"dir/sub/file.jpg":      "file.jpg",
		"a\x01b\x7f.png":        "ab.png",
		"":                      "file",
		".":                     "file",
		"..":                    "file",
		"/":                     "file",
	}
	for in, want := range cases {
		if got := SafeFilename(in); got != want {
			t.Fatalf("SafeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUploadToS3(t *testing.T) {
	t.Setenv("AWS_BUCKET", "test-bucket")
	t.Setenv("AWS_REGION", "ap-southeast-1")
	t.Setenv("AWS_ACCESS_KEY", "key")
	t.Setenv("AWS_SECRET_KEY", "secret")

	t.Run("puts object under the prefix", func(t *testing.T) {
		var gotPath string
		var gotBody []byte
		var gotContentType string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotContentType = r.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		t.Setenv("AWS_ENDPOINT", server.URL)

		header := fileHeader(t, "pic.png", []byte("hello"))
		filePath, err := UploadToS3(header, "app-key/2026/01/01/ABCDE/")
		if err != nil {
			t.Fatalf("UploadToS3: %v", err)
		}
		if want := "app-key/2026/01/01/ABCDE/pic.png"; filePath != want {
			t.Fatalf("filePath = %q, want %q", filePath, want)
		}

		if want := "/test-bucket/app-key/2026/01/01/ABCDE/pic.png"; gotPath != want {
			t.Fatalf("path = %q, want %q", gotPath, want)
		}
		if string(gotBody) != "hello" {
			t.Fatalf("body = %q, want %q", gotBody, "hello")
		}
		if !strings.HasPrefix(gotContentType, "text/plain") {
			t.Fatalf("content type = %q, want text/plain*", gotContentType)
		}
	})

	t.Run("returns upload errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		t.Setenv("AWS_ENDPOINT", server.URL)

		header := fileHeader(t, "pic.png", []byte("hello"))
		filePath, err := UploadToS3(header, "app-key/")
		if err == nil {
			t.Fatal("UploadToS3 returned nil for a failing endpoint")
		}
		if filePath != "" {
			t.Fatalf("filePath = %q, want empty on failure", filePath)
		}
	})

	t.Run("sanitises the object key", func(t *testing.T) {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		t.Setenv("AWS_ENDPOINT", server.URL)

		header := fileHeader(t, "../evil.png", []byte("hello"))
		filePath, err := UploadToS3(header, "app-key/2026/01/01/ABCDE/")
		if err != nil {
			t.Fatalf("UploadToS3: %v", err)
		}
		if want := "app-key/2026/01/01/ABCDE/evil.png"; filePath != want {
			t.Fatalf("filePath = %q, want %q", filePath, want)
		}

		if want := "/test-bucket/app-key/2026/01/01/ABCDE/evil.png"; gotPath != want {
			t.Fatalf("path = %q, want %q", gotPath, want)
		}
	})
}

func TestUploadToS3FatalOnMissingConfig(t *testing.T) {
	runFatalCase(t, "UPLASS_HELPER_S3_NO_CONFIG", "Invalid AWS settings", func() {
		for _, key := range []string{"AWS_BUCKET", "AWS_REGION", "AWS_ACCESS_KEY", "AWS_SECRET_KEY"} {
			os.Unsetenv(key)
		}
		_, _ = UploadToS3(&multipart.FileHeader{}, "prefix/")
	})
}
