package utils

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGetGDBFatalOnMissingConnString(t *testing.T) {
	runFatalCase(t, "UPLASS_HELPER_GETGDB_NO_CONN", "Invalid PGSQL settings", func() {
		os.Unsetenv("PGSQL_CONN_STR")
		os.Setenv("PGSQL_MAX_OPEN_CONNS", "5")
		GetGDB()
	})
}

func TestGetGDBFatalOnBadMaxConns(t *testing.T) {
	runFatalCase(t, "UPLASS_HELPER_GETGDB_BAD_CONNS", "Invalid PGSQL settings", func() {
		os.Setenv("PGSQL_CONN_STR", "postgres://user:pass@localhost:5432/db")
		os.Setenv("PGSQL_MAX_OPEN_CONNS", "not-a-number")
		GetGDB()
	})
}

// runFatalCase reruns the test binary and expects log.Fatal inside fn
func runFatalCase(t *testing.T, helperEnv string, wantOutput string, fn func()) {
	t.Helper()

	if os.Getenv(helperEnv) == "1" {
		fn()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = []string{helperEnv + "=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, helperEnv+"=") {
			continue
		}
		// Hide PGSQL_/AWS_ settings the function under test would read
		if strings.HasPrefix(kv, "PGSQL_") || strings.HasPrefix(kv, "AWS_") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a fatal exit, got success. output: %s", out)
	}
	if !strings.Contains(string(out), wantOutput) {
		t.Fatalf("output = %q, want it to contain %q", out, wantOutput)
	}
}
