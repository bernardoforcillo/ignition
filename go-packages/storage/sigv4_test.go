package storage

import (
	"strings"
	"testing"
	"time"
)

// TestS3Sign_MatchesTheAWSDocumentedExample pins the SigV4 implementation to the one published
// example (the "GET /test.txt" presigned URL in AWS's query-string authentication guide), so a
// self-consistent but wrong canonical form cannot pass the round-trip tests against our own fake.
func TestS3Sign_MatchesTheAWSDocumentedExample(t *testing.T) {
	s, err := NewS3(S3Config{
		Bucket: "examplebucket", Region: "us-east-1", Endpoint: "https://s3.amazonaws.com",
		AccessKeyID: "AKIAIOSFODNN7EXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.sign(request{method: "GET", key: "test.txt", expires: 86400 * time.Second},
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := "https://examplebucket.s3.amazonaws.com/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256" +
		"&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host" +
		"&X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if got.URL != want {
		t.Errorf("url =\n%s\nwant\n%s", got.URL, want)
	}
}

func TestS3Locate_Styles(t *testing.T) {
	cases := []struct {
		name       string
		cfg        S3Config
		host, path string
	}{
		{"aws virtual hosted", S3Config{}, "b.s3.eu-west-1.amazonaws.com", "/k"},
		{"path style", S3Config{Endpoint: "http://minio:9000", PathStyle: true}, "minio:9000", "/b/k"},
		{"virtual style on a custom endpoint", S3Config{Endpoint: "https://r2.example.com"}, "b.r2.example.com", "/k"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Bucket, tc.cfg.Region, tc.cfg.AccessKeyID, tc.cfg.SecretAccessKey = "b", "eu-west-1", "id", "secret"
			s, err := NewS3(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			_, host, path := s.locate("k")
			if host != tc.host || path != tc.path {
				t.Errorf("locate = %q %q, want %q %q", host, path, tc.host, tc.path)
			}
		})
	}
}

func TestContentDisposition_NeverCarriesQuotesOrControlCharacters(t *testing.T) {
	got := contentDisposition("re\"port\r\n;é.pdf")
	if strings.ContainsAny(got, "\r\n") || strings.Count(got, `"`) != 2 {
		t.Errorf("unsafe header: %q", got)
	}
	if !strings.Contains(got, "filename*=UTF-8''re%22port%3B%C3%A9.pdf") {
		t.Errorf("RFC 5987 form missing or wrong: %q", got)
	}
}

func TestValidateKey(t *testing.T) {
	for key, ok := range map[string]bool{
		"workspaces/w/obj": true, "a": true,
		"": false, "/abs": false, "a//b": false, "a/../b": false, "a/./b": false, "a\nb": false,
		strings.Repeat("x", 1025): false,
	} {
		if got := validateKey(key) == nil; got != ok {
			t.Errorf("validateKey(%q) ok = %v, want %v", key, got, ok)
		}
	}
}
