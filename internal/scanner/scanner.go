package scanner

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// KnownPattern represents a file pattern that typically contains credentials
type KnownPattern struct {
	Pattern     string // glob pattern relative to project root
	Category    string // grouping label (e.g. "Environment", "PHP", "Docker")
	Description string // short explanation
}

// DetectedFile is a file that matched a known credential pattern
type DetectedFile struct {
	Path        string // relative path from project root
	Category    string
	Description string
}

// knownPatterns are common credential file locations across frameworks
var knownPatterns = []KnownPattern{
	// Environment files
	{".env", "Environment", "Environment variables"},
	{".env.local", "Environment", "Local environment overrides"},
	{".env.production", "Environment", "Production environment"},
	{".env.staging", "Environment", "Staging environment"},
	{".env.development", "Environment", "Development environment"},

	// PHP / Laravel
	{"phpunit.xml", "PHP", "PHPUnit config (may contain DB credentials)"},
	{"auth.json", "PHP", "Composer authentication"},

	// Node.js
	{".npmrc", "Node.js", "npm registry auth tokens"},
	{".yarnrc.yml", "Node.js", "Yarn config with tokens"},

	// Ruby / Rails
	{"config/secrets.yml", "Ruby", "Rails secrets"},
	{"config/database.yml", "Ruby", "Rails database credentials"},
	{"config/master.key", "Ruby", "Rails master key"},

	// Python / Django
	{"local_settings.py", "Python", "Django local settings"},
	{".pypirc", "Python", "PyPI credentials"},

	// Java / Spring
	{"application.properties", "Java", "Spring config"},
	{"application.yml", "Java", "Spring config (YAML)"},
	{"src/main/resources/application.properties", "Java", "Spring config"},
	{"src/main/resources/application.yml", "Java", "Spring config (YAML)"},

	// .NET
	{"appsettings.json", ".NET", "ASP.NET configuration"},
	{"appsettings.Development.json", ".NET", "ASP.NET dev configuration"},

	// Docker
	{"docker-compose.yml", "Docker", "Docker Compose (may contain passwords)"},
	{"compose.yml", "Docker", "Docker Compose (may contain passwords)"},
	{"docker-compose.override.yml", "Docker", "Docker Compose overrides"},

	// Infrastructure
	{"terraform.tfvars", "Infrastructure", "Terraform variables"},
	{"terraform.tfstate", "Infrastructure", "Terraform state (contains secrets)"},

	// Cloud / CI
	{".aws/credentials", "Cloud", "AWS credentials"},
	{"gcloud-service-account.json", "Cloud", "GCP service account"},
	{"service-account.json", "Cloud", "GCP service account"},
	{"firebase.json", "Cloud", "Firebase configuration"},

	// WordPress
	{"wp-config.php", "WordPress", "WordPress database credentials"},

	// Certificates / Keys
	{"*.pem", "Certificates", "PEM certificate/key"},
	{"*.key", "Certificates", "Private key"},

	// Generic
	{"secrets.json", "Generic", "Secrets file"},
	{"secrets.yml", "Generic", "Secrets file"},
	{"secrets.yaml", "Generic", "Secrets file"},
	{"credentials.json", "Generic", "Credentials file"},
	{".htpasswd", "Generic", "HTTP basic auth passwords"},
}

// skipDirs are not worth scanning and routinely contain unrelated key/cert
// files that would be false positives.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true, "dist": true,
	"build": true, ".next": true, ".nuxt": true, ".venv": true, "venv": true,
	"target": true, ".terraform": true, ".cache": true,
}

// Detect walks the project tree for known credential file patterns. Basename
// patterns (".env", "*.pem") match at any depth so monorepo files like
// apps/web/.env are found; path patterns ("config/database.yml") match by path
// suffix. Returns only regular files that exist on disk.
func Detect(root string) ([]DetectedFile, error) {
	var found []DetectedFile
	seen := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || seen[rel] {
			return nil
		}
		for _, kp := range knownPatterns {
			var matched bool
			if strings.ContainsRune(kp.Pattern, '/') {
				matched = rel == kp.Pattern || strings.HasSuffix(rel, string(filepath.Separator)+kp.Pattern)
			} else {
				matched, _ = filepath.Match(kp.Pattern, d.Name())
			}
			if matched {
				seen[rel] = true
				found = append(found, DetectedFile{
					Path:        rel,
					Category:    kp.Category,
					Description: kp.Description,
				})
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Category != found[j].Category {
			return found[i].Category < found[j].Category
		}
		return found[i].Path < found[j].Path
	})

	return found, nil
}
