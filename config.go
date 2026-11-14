package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var requiredSettings = []string{
	"PGSQL_CONN_STR",
	"AWS_BUCKET",
	"AWS_REGION",
	"AWS_ACCESS_KEY",
	"AWS_SECRET_KEY",
	"ASSET_HOST",
}

// validateConfig fails at boot so a misconfigured instance never passes its
// health check and then dies on the first real request
func validateConfig() error {
	for _, key := range requiredSettings {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return fmt.Errorf("missing %s", key)
		}
	}

	maxOpenConns, err := strconv.Atoi(strings.TrimSpace(os.Getenv("PGSQL_MAX_OPEN_CONNS")))
	if err != nil || maxOpenConns <= 0 {
		return errors.New("PGSQL_MAX_OPEN_CONNS must be a positive integer")
	}

	assetHost, err := url.Parse(os.Getenv("ASSET_HOST"))
	if err != nil || assetHost.Host == "" ||
		(assetHost.Scheme != "http" && assetHost.Scheme != "https") {
		return errors.New("ASSET_HOST must be an absolute http(s) URL")
	}

	return nil
}
