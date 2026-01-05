package presets

import "github.com/IRevolve/Bear/internal/config"

// Languages enthält alle vordefinierten Sprach-Konfigurationen
var Languages = map[string]config.Language{
	"go": {
		Name: "go",
		Detection: config.Detection{
			Files: []string{"go.mod"},
		},
		Validation: config.Validation{
			Setup: []config.Step{
				{Name: "Download modules", Run: "go mod download"},
			},
			Lint: []config.Step{
				{Name: "Vet", Run: "go vet ./..."},
			},
			Test: []config.Step{
				{Name: "Test", Run: "go test -race ./..."},
			},
			Build: []config.Step{
				{Name: "Build", Run: "go build -o dist/app ."},
			},
		},
	},
	"node": {
		Name: "node",
		Detection: config.Detection{
			Files: []string{"package.json"},
		},
		Validation: config.Validation{
			Setup: []config.Step{
				{Name: "Install", Run: "npm ci"},
			},
			Lint: []config.Step{
				{Name: "Lint", Run: "npm run lint --if-present"},
			},
			Test: []config.Step{
				{Name: "Test", Run: "npm test --if-present"},
			},
			Build: []config.Step{
				{Name: "Build", Run: "npm run build --if-present"},
			},
		},
	},
	"python": {
		Name: "python",
		Detection: config.Detection{
			Files: []string{"requirements.txt", "pyproject.toml", "setup.py"},
		},
		Validation: config.Validation{
			Setup: []config.Step{
				{Name: "Install", Run: "pip install -r requirements.txt"},
			},
			Lint: []config.Step{
				{Name: "Lint", Run: "ruff check . || pylint **/*.py || true"},
			},
			Test: []config.Step{
				{Name: "Test", Run: "pytest || python -m unittest discover || true"},
			},
			Build: []config.Step{
				{Name: "Build", Run: "echo 'No build step for Python'"},
			},
		},
	},
	"rust": {
		Name: "rust",
		Detection: config.Detection{
			Files: []string{"Cargo.toml"},
		},
		Validation: config.Validation{
			Setup: []config.Step{
				{Name: "Fetch", Run: "cargo fetch"},
			},
			Lint: []config.Step{
				{Name: "Clippy", Run: "cargo clippy -- -D warnings"},
			},
			Test: []config.Step{
				{Name: "Test", Run: "cargo test"},
			},
			Build: []config.Step{
				{Name: "Build", Run: "cargo build --release"},
			},
		},
	},
	"java": {
		Name: "java",
		Detection: config.Detection{
			Files: []string{"pom.xml", "build.gradle", "build.gradle.kts"},
		},
		Validation: config.Validation{
			Setup: []config.Step{
				{Name: "Download", Run: "mvn dependency:go-offline || gradle dependencies"},
			},
			Lint: []config.Step{
				{Name: "Check", Run: "mvn checkstyle:check || gradle check || true"},
			},
			Test: []config.Step{
				{Name: "Test", Run: "mvn test || gradle test"},
			},
			Build: []config.Step{
				{Name: "Build", Run: "mvn package -DskipTests || gradle build -x test"},
			},
		},
	},
	"typescript": {
		Name: "typescript",
		Detection: config.Detection{
			Files: []string{"tsconfig.json"},
		},
		Validation: config.Validation{
			Setup: []config.Step{
				{Name: "Install", Run: "npm ci"},
			},
			Lint: []config.Step{
				{Name: "Type check", Run: "npx tsc --noEmit"},
				{Name: "Lint", Run: "npm run lint --if-present"},
			},
			Test: []config.Step{
				{Name: "Test", Run: "npm test --if-present"},
			},
			Build: []config.Step{
				{Name: "Build", Run: "npm run build"},
			},
		},
	},
}

// GetLanguage returns a predefined language
func GetLanguage(name string) (config.Language, bool) {
	lang, ok := Languages[name]
	return lang, ok
}

// ListLanguages returns all available languages
func ListLanguages() []string {
	var names []string
	for name := range Languages {
		names = append(names, name)
	}
	return names
}
