package common

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/samber/lo"
)

var (
	maskURLPattern    = regexp.MustCompile(`(http|https)://[^\s/$.?#].[^\s]*`)
	maskDomainPattern = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`)
	maskIPPattern     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// credentialLabelPattern catches "api_key: <value>" style text that providers
	// echo back in error messages; credentialTokenPattern catches key shapes that
	// appear bare (sk-..., GitHub PATs, Slack tokens, JWTs).
	credentialLabelPattern = regexp.MustCompile(`(?i)\b(?:bearer\s+|(?:api[_ -]?key|access[_ -]?token|authorization)\s*[=:]\s*["']?(?:(?:bearer|basic)\s+)?)[^\s"',;<>}]+`)
	credentialTokenPattern = regexp.MustCompile(`(?i)\b(?:sk|ghp|gho|ghu|ghs|ghr|xox[baprs])[-_][-A-Za-z0-9_]{10,}\b|\bgithub_pat_[-A-Za-z0-9_]{16,}\b|\beyJ[-A-Za-z0-9_]{8,}(?:\.[-A-Za-z0-9_]{4,}){2}\b`)

	redactedPlaceholder = "[redacted]"

	// maskApiKeyPattern matches patterns like 'api_key:xxx' or "api_key:xxx" to mask the API key value
	maskApiKeyPattern = regexp.MustCompile(`(['"]?)api_key:([^\s'"]+)(['"]?)`)

	// urlCredentialsPattern matches the userinfo section of an http(s) URL, i.e. the
	// "user:token@" part between the scheme and the host. Only http(s) is matched so
	// mailto: addresses and other text keep their "@" instead of being masked.
	urlCredentialsPattern = regexp.MustCompile(`(?i)\b(https?://)[^\s/@]+@`)
)

// RedactSecrets strips credentials that a provider or driver echoes back into an
// error message, a log line or a stored status reason. Literal secrets go first
// because they are the only ones known for certain, then credential-shaped text.
func RedactSecrets(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			value = strings.ReplaceAll(value, secret, redactedPlaceholder)
		}
	}
	value = credentialLabelPattern.ReplaceAllString(value, redactedPlaceholder)
	return credentialTokenPattern.ReplaceAllString(value, redactedPlaceholder)
}

// RedactURLCredentials replaces the userinfo of any URL with "***", turning
// https://user:token@host/repo into https://***@host/repo. Tools that echo a
// configured remote back (git prints it in fetch/pull output and error text)
// would otherwise leak the access token embedded in the URL.
func RedactURLCredentials(value string) string {
	return urlCredentialsPattern.ReplaceAllString(value, "${1}***@")
}

// TruncateForLog bounds a captured upstream body so a single malformed response
// cannot bloat a log file or a client error, while keeping the head useful for
// diagnosis. Secrets are stripped before cutting, so a truncated tail cannot
// leave a partial credential behind.
func TruncateForLog(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

func GetStringIfEmpty(str string, defaultValue string) string {
	if str == "" {
		return defaultValue
	}
	return str
}

func GetRandomString(length int) string {
	if length <= 0 {
		return ""
	}
	return lo.RandomString(length, lo.AlphanumericCharset)
}

func MapToJsonStr(m map[string]interface{}) string {
	bytes, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func StrToMap(str string) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	err := Unmarshal([]byte(str), &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func StrToJsonArray(str string) ([]interface{}, error) {
	var js []interface{}
	err := json.Unmarshal([]byte(str), &js)
	if err != nil {
		return nil, err
	}
	return js, nil
}

func IsJsonArray(str string) bool {
	var js []interface{}
	return json.Unmarshal([]byte(str), &js) == nil
}

func IsJsonObject(str string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(str), &js) == nil
}

func String2Int(str string) int {
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return num
}

func StringsContains(strs []string, str string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

// StringToByteSlice []byte only read, panic on append
func StringToByteSlice(s string) []byte {
	tmp1 := (*[2]uintptr)(unsafe.Pointer(&s))
	tmp2 := [3]uintptr{tmp1[0], tmp1[1], tmp1[1]}
	return *(*[]byte)(unsafe.Pointer(&tmp2))
}

func EncodeBase64(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

func GetJsonString(data any) string {
	if data == nil {
		return ""
	}
	b, _ := json.Marshal(data)
	return string(b)
}

// NormalizeBillingPreference clamps the billing preference to valid values.
func NormalizeBillingPreference(pref string) string {
	switch strings.TrimSpace(pref) {
	case "subscription_first", "wallet_first", "subscription_only", "wallet_only":
		return strings.TrimSpace(pref)
	default:
		return "subscription_first"
	}
}

// MaskEmail masks a user email to prevent PII leakage in logs
// Returns "***masked***" if email is empty, otherwise shows only the domain part
func MaskEmail(email string) string {
	if email == "" {
		return "***masked***"
	}

	// Find the @ symbol
	atIndex := strings.Index(email, "@")
	if atIndex == -1 {
		// No @ symbol found, return masked
		return "***masked***"
	}

	// Return only the domain part with @ symbol
	return "***@" + email[atIndex+1:]
}

// maskHostTail returns the tail parts of a domain/host that should be preserved.
// It keeps 2 parts for likely country-code TLDs (e.g., co.uk, com.cn), otherwise keeps only the TLD.
func maskHostTail(parts []string) []string {
	if len(parts) < 2 {
		return parts
	}
	lastPart := parts[len(parts)-1]
	secondLastPart := parts[len(parts)-2]
	if len(lastPart) == 2 && len(secondLastPart) <= 3 {
		// Likely country code TLD like co.uk, com.cn
		return []string{secondLastPart, lastPart}
	}
	return []string{lastPart}
}

// maskHostForURL collapses subdomains and keeps only masked prefix + preserved tail.
// Example: api.openai.com -> ***.com, sub.domain.co.uk -> ***.co.uk
func maskHostForURL(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "***"
	}
	tail := maskHostTail(parts)
	return "***." + strings.Join(tail, ".")
}

// maskHostForPlainDomain masks a plain domain and reflects subdomain depth with multiple ***.
// Example: openai.com -> ***.com, api.openai.com -> ***.***.com, sub.domain.co.uk -> ***.***.co.uk
func maskHostForPlainDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}
	tail := maskHostTail(parts)
	numStars := len(parts) - len(tail)
	if numStars < 1 {
		numStars = 1
	}
	stars := strings.TrimSuffix(strings.Repeat("***.", numStars), ".")
	return stars + "." + strings.Join(tail, ".")
}

// MaskSensitiveInfo masks sensitive information like URLs, IPs, and domain names in a string
// Example:
// http://example.com -> http://***.com
// https://api.test.org/v1/users/123?key=secret -> https://***.org/***/***/?key=***
// https://sub.domain.co.uk/path/to/resource -> https://***.co.uk/***/***
// 192.168.1.1 -> ***.***.***.***
// openai.com -> ***.com
// www.openai.com -> ***.***.com
// api.openai.com -> ***.***.com
func MaskSensitiveInfo(str string) string {
	// Mask URLs
	str = maskURLPattern.ReplaceAllStringFunc(str, func(urlStr string) string {
		u, err := url.Parse(urlStr)
		if err != nil {
			return urlStr
		}

		host := u.Host
		if host == "" {
			return urlStr
		}

		// Mask host with unified logic
		maskedHost := maskHostForURL(host)

		result := u.Scheme + "://" + maskedHost

		// Mask path
		if u.Path != "" && u.Path != "/" {
			pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
			maskedPathParts := make([]string, len(pathParts))
			for i := range pathParts {
				if pathParts[i] != "" {
					maskedPathParts[i] = "***"
				}
			}
			if len(maskedPathParts) > 0 {
				result += "/" + strings.Join(maskedPathParts, "/")
			}
		} else if u.Path == "/" {
			result += "/"
		}

		// Mask query parameters
		if u.RawQuery != "" {
			values, err := url.ParseQuery(u.RawQuery)
			if err != nil {
				// If can't parse query, just mask the whole query string
				result += "?***"
			} else {
				maskedParams := make([]string, 0, len(values))
				for key := range values {
					maskedParams = append(maskedParams, key+"=***")
				}
				if len(maskedParams) > 0 {
					result += "?" + strings.Join(maskedParams, "&")
				}
			}
		}

		return result
	})

	// Mask domain names without protocol (like openai.com, www.openai.com)
	str = maskDomainPattern.ReplaceAllStringFunc(str, func(domain string) string {
		return maskHostForPlainDomain(domain)
	})

	// Mask IP addresses
	str = maskIPPattern.ReplaceAllString(str, "***.***.***.***")

	// Mask API keys (e.g., "api_key:AIzaSyAAAaUooTUni8AdaOkSRMda30n_Q4vrV70" -> "api_key:***")
	str = maskApiKeyPattern.ReplaceAllString(str, "${1}api_key:***${3}")

	return str
}
