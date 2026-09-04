package openapi

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Public Seedance documentation is an externally visible contract. Keep the
// internal enhancement topology and service-to-service APIs out of both the
// OpenAPI source of truth and the Apifox supplements that are published from it.
func TestPublicSeedanceDocumentationContainsNoPrivateWorkflowTerms(t *testing.T) {
	paths := []string{"public.json"}
	apifoxPaths, err := filepath.Glob(filepath.Join("..", "apifox", "seedance", "*"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, apifoxPaths...)

	forbidden := map[string]*regexp.Regexp{
		"BYOK":                    regexp.MustCompile(`(?i)\bbyok\b`),
		"enhancement":             regexp.MustCompile(`(?i)\benhancement\b`),
		"super_resolution":        regexp.MustCompile(`(?i)super[-_ ]?resolution`),
		"upscale":                 regexp.MustCompile(`(?i)\bupscale\b`),
		"SR":                      regexp.MustCompile(`(?i)\bsr\b`),
		"Chinese internal terms":  regexp.MustCompile(`超分|增强`),
		"provider type":           regexp.MustCompile(`(?i)\bprovider_type\b`),
		"service code":            regexp.MustCompile(`(?i)\bservice_code\b`),
		"execution task id":       regexp.MustCompile(`(?i)\bexecution_task_id\b`),
		"internal provider enums": regexp.MustCompile(`(?i)\b(?:AIPDD_INTERNAL|DIRECT_EXTERNAL|VOLCENGINE_MEDIAKIT)\b`),
		"finance service route":   regexp.MustCompile(`(?i)/api/finance/`),
		"media service route":     regexp.MustCompile(`(?i)/api/media/`),
	}

	for _, path := range paths {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		for label, pattern := range forbidden {
			if pattern.Match(body) {
				t.Errorf("%s exposes forbidden %s content", path, label)
			}
		}
	}
}
