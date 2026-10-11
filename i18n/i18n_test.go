package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// templateVarPattern captures the {{.Name}} placeholders of a message template.
var templateVarPattern = regexp.MustCompile(`\{\{\.([A-Za-z0-9_]+)\}\}`)

// TestEveryKeyIsTranslated guards the two ways a new message can silently break:
// a constant in keys.go with no entry (T() then renders the raw dotted key to the
// operator) and a translation that drops or renames a {{.Placeholder}}.
func TestEveryKeyIsTranslated(t *testing.T) {
	keys := declaredMessageKeys(t)
	require.NotEmpty(t, keys)

	base := readLocaleMessages(t, LangEn)
	for _, lang := range SupportedLanguages() {
		messages := readLocaleMessages(t, lang)
		for _, key := range keys {
			_, ok := messages[key]
			require.Truef(t, ok, "%s is missing from locales/%s.yaml", key, lang)
		}
		if lang == LangEn {
			continue
		}
		for _, key := range keys {
			require.Equalf(t, templateVars(base[key]), templateVars(messages[key]),
				"locales/%s.yaml message %q has different placeholders than en.yaml", lang, key)
		}
	}
}

func declaredMessageKeys(t *testing.T) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "keys.go", nil, 0)
	require.NoError(t, err)

	seen := make(map[string]bool)
	var keys []string
	for _, decl := range parsed.Decls {
		value, ok := decl.(*ast.GenDecl)
		if !ok || value.Tok != token.CONST {
			continue
		}
		for _, spec := range value.Specs {
			constSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, literal := range constSpec.Values {
				basic, ok := literal.(*ast.BasicLit)
				if !ok || basic.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(basic.Value)
				require.NoError(t, err)
				if !strings.Contains(text, ".") || seen[text] {
					continue
				}
				seen[text] = true
				keys = append(keys, text)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func readLocaleMessages(t *testing.T, lang string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("locales", lang+".yaml"))
	require.NoError(t, err)
	messages := make(map[string]string)
	require.NoError(t, yaml.Unmarshal(raw, &messages))
	return messages
}

func templateVars(message string) []string {
	var vars []string
	for _, match := range templateVarPattern.FindAllStringSubmatch(message, -1) {
		vars = append(vars, match[1])
	}
	sort.Strings(vars)
	return vars
}
