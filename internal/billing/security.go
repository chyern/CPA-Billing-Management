package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// MaskAPIKey returns a recognizable but non-secret API key label. The full key
// is deliberately discarded before a usage event reaches persistent storage.
func MaskAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	switch {
	case len(runes) <= 2:
		return strings.Repeat("•", len(runes))
	case len(runes) <= 8:
		return string(runes[:1]) + strings.Repeat("•", len(runes)-2) + string(runes[len(runes)-1:])
	default:
		return string(runes[:4]) + "••••••" + string(runes[len(runes)-4:])
	}
}

// APIKeyIdentifier separates credentials that happen to have the same masked
// label without retaining the full key. It is used only as a grouping key.
func APIKeyIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:8])
}

// CallerScope matches CLIProxyAPI's irreversible downstream-caller identity.
// Keeping this derivation here lets the management page store the host identity
// alongside a balance without persisting the complete API key.
func CallerScope(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("cli-proxy-api:caller-scope:v1\x00" + value))
	return hex.EncodeToString(sum[:])
}

// MaskSensitiveSource protects source values that are actually credentials in
// some CPA integrations, while preserving ordinary source labels.
func MaskSensitiveSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '•') {
		return value
	}
	lower := strings.ToLower(value)
	secretLike := strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "rk-") || strings.HasPrefix(lower, "pk-") || strings.HasPrefix(lower, "api-") || strings.HasPrefix(lower, "bearer ")
	if secretLike {
		return MaskAPIKey(value)
	}
	return value
}

func nonNegativeDuration(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value)
}
