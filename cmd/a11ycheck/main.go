package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/a11y"
)

type pageReport struct {
	URL        string           `json:"url"`
	FinalURL   string           `json:"final_url,omitempty"`
	Status     int              `json:"status"`
	Violations []a11y.Violation `json:"violations"`
}

type pathsFlag []string

func (p *pathsFlag) String() string { return strings.Join(*p, ",") }

func (p *pathsFlag) Set(value string) error {
	*p = append(*p, value)
	return nil
}

func main() {
	baseURL := flag.String("base", envOr("GOFER_BASE_URL", "http://127.0.0.1:8090"), "base URL of the running Gofer instance")
	cookie := flag.String("cookie", os.Getenv("GOFER_COOKIE"), "Cookie header value for authenticated pages")
	jsonOutput := flag.Bool("json", false, "print a JSON report")
	var paths pathsFlag
	flag.Var(&paths, "path", "path to audit; repeat for multiple pages")
	flag.Parse()
	if len(paths) == 0 {
		paths = append(paths, "/")
	}
	base, err := url.Parse(strings.TrimRight(*baseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		fmt.Fprintf(os.Stderr, "a11ycheck: invalid base URL %q\n", *baseURL)
		os.Exit(2)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	var reports []pageReport
	failed := false
	for _, path := range paths {
		target := base.ResolveReference(&url.URL{Path: path})
		request, err := http.NewRequest(http.MethodGet, target.String(), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "a11ycheck: %v\n", err)
			failed = true
			continue
		}
		request.Header.Set("User-Agent", "gofer-a11ycheck/1")
		if *cookie != "" {
			request.Header.Set("Cookie", *cookie)
		}
		response, err := client.Do(request)
		if err != nil {
			fmt.Fprintf(os.Stderr, "a11ycheck: %s: %v\n", target, err)
			failed = true
			continue
		}
		report := pageReport{URL: target.String(), Status: response.StatusCode, FinalURL: response.Request.URL.String()}
		if response.StatusCode != http.StatusOK {
			report.Violations = append(report.Violations, a11y.Violation{
				Rule:    "http-status",
				Message: fmt.Sprintf("expected 200, got %d", response.StatusCode),
			})
		} else {
			audit, auditErr := a11y.Audit(io.LimitReader(response.Body, 8<<20))
			if auditErr != nil {
				fmt.Fprintf(os.Stderr, "a11ycheck: %s: %v\n", target, auditErr)
				failed = true
			} else {
				report.Violations = audit.Violations
			}
		}
		_ = response.Body.Close()
		if len(report.Violations) > 0 {
			failed = true
		}
		reports = append(reports, report)
	}

	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(reports)
	} else {
		for _, report := range reports {
			fmt.Printf("%s: %d violation(s)\n", report.URL, len(report.Violations))
			if report.FinalURL != "" && report.FinalURL != report.URL {
				fmt.Printf("  redirected to %s\n", report.FinalURL)
			}
			for _, violation := range report.Violations {
				fmt.Printf("  [%s] %s\n", violation.Rule, violation.Message)
			}
		}
	}
	if failed {
		os.Exit(1)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
