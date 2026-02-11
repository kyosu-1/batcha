package batcha

import (
	"strings"
	"unicode"
)

// skipConvertKeys are map keys (lowercase) whose children should NOT have
// their keys converted, because they are user-defined (e.g., tag keys, parameters).
var skipConvertKeys = map[string]bool{
	"options":    true,
	"parameters": true,
	"tags":       true,
}

// walkMap recursively converts map keys using the provided function.
func walkMap(v any, fn func(string) string) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, child := range val {
			newKey := fn(k)
			if skipConvertKeys[strings.ToLower(k)] {
				result[newKey] = child
			} else {
				result[newKey] = walkMap(child, fn)
			}
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, child := range val {
			result[i] = walkMap(child, fn)
		}
		return result
	default:
		return v
	}
}

// toPascalCase converts a camelCase string to PascalCase.
func toPascalCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// toCamelCase converts a PascalCase string to camelCase.
func toCamelCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}
