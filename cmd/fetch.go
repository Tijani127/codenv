package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func fetchBytes(ref string) ([]byte, error) {
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(ref)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", ref, resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	abs, err := filepath.Abs(ref)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}
