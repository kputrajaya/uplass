package main

import "testing"

func TestValidateConfig(t *testing.T) {
	t.Run("complete config", func(t *testing.T) {
		setValidConfig(t)
		if err := validateConfig(); err != nil {
			t.Fatalf("validateConfig() = %v, want nil", err)
		}
	})

	t.Run("asset host without trailing slash", func(t *testing.T) {
		setValidConfig(t)
		t.Setenv("ASSET_HOST", "https://cdn.test")
		if err := validateConfig(); err != nil {
			t.Fatalf("validateConfig() = %v, want nil", err)
		}
	})

	for _, key := range requiredSettings {
		t.Run("missing "+key, func(t *testing.T) {
			setValidConfig(t)
			t.Setenv(key, "")
			if err := validateConfig(); err == nil {
				t.Fatalf("validateConfig() = nil, want an error for a missing %s", key)
			}
		})
	}

	t.Run("bad connection limit", func(t *testing.T) {
		for _, value := range []string{"0", "-1", "not-a-number"} {
			t.Run(value, func(t *testing.T) {
				setValidConfig(t)
				t.Setenv("PGSQL_MAX_OPEN_CONNS", value)
				if err := validateConfig(); err == nil {
					t.Fatalf("validateConfig() = nil, want an error for %q", value)
				}
			})
		}
	})

	t.Run("relative asset host", func(t *testing.T) {
		for _, value := range []string{"cdn.test/", "ftp://cdn.test/", "https://"} {
			t.Run(value, func(t *testing.T) {
				setValidConfig(t)
				t.Setenv("ASSET_HOST", value)
				if err := validateConfig(); err == nil {
					t.Fatalf("validateConfig() = nil, want an error for %q", value)
				}
			})
		}
	})
}

func setValidConfig(t *testing.T) {
	t.Helper()

	settings := map[string]string{
		"PGSQL_CONN_STR":       "postgres://user:pass@host:5432/db",
		"PGSQL_MAX_OPEN_CONNS": "10",
		"AWS_BUCKET":           "bucket",
		"AWS_REGION":           "ap-southeast-1",
		"AWS_ACCESS_KEY":       "key",
		"AWS_SECRET_KEY":       "secret",
		"ASSET_HOST":           "https://cdn.test/",
	}
	for key, value := range settings {
		t.Setenv(key, value)
	}
}
