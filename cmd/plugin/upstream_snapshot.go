package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"strings"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
	"gopkg.in/yaml.v3"
)

// Only the display snapshot is persisted. Credentials and routing identifiers
// are used during ingestion and are never copied into the event table.
var hostConfigPath string

type upstreamConfigEntry struct {
	Name          string `yaml:"name"`
	APIKey        string `yaml:"api-key"`
	BaseURL       string `yaml:"base-url"`
	APIKeyEntries []struct {
		APIKey string `yaml:"api-key"`
	} `yaml:"api-key-entries"`
}

func usageDomainSnapshot(record billing.UsageRecord) string {
	if hostConfigPath == "" || strings.TrimSpace(record.AuthIndex) == "" {
		return ""
	}
	raw, err := os.ReadFile(hostConfigPath)
	if err != nil {
		return ""
	}
	return domainSnapshotFromConfig(record, raw)
}

func domainSnapshotFromConfig(record billing.UsageRecord, raw []byte) string {
	var config map[string]yaml.Node
	if yaml.Unmarshal(raw, &config) != nil {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(record.Provider))
	index := strings.TrimSpace(record.AuthIndex)
	if index == "" {
		return ""
	}
	matchedDomain := ""
	for _, section := range []string{"codex-api-key", "claude-api-key", "gemini-api-key", "xai-api-key", "interactions-api-key", "openai-compatibility"} {
		node, exists := config[section]
		if !exists {
			continue
		}
		var entries []upstreamConfigEntry
		if node.Decode(&entries) != nil {
			return ""
		}
		for _, entry := range entries {
			entryProvider := strings.TrimSuffix(section, "-api-key")
			if section == "interactions-api-key" {
				entryProvider = "gemini-interactions"
			}
			if section == "openai-compatibility" {
				entryProvider = strings.ToLower(strings.TrimSpace(entry.Name))
				if entryProvider == "" {
					entryProvider = "openai-compatibility"
				}
			}
			if entryProvider != provider && !(section == "openai-compatibility" && "openai-compatible-"+entryProvider == provider) {
				continue
			}
			keys := []string{entry.APIKey}
			if section == "openai-compatibility" {
				keys = nil
				for _, entryKey := range entry.APIKeyEntries {
					keys = append(keys, entryKey.APIKey)
				}
			}
			for _, key := range keys {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				base := strings.TrimSpace(entry.BaseURL)
				// CLIProxyAPI v7 uses this credential identity for config-backed API keys.
				// Matching the complete index fails closed if the host's scheme changes.
				digest := sha256.Sum256([]byte(section + ":" + base + "+" + key))
				if hex.EncodeToString(digest[:8]) != index {
					continue
				}
				parsed, err := url.Parse(base)
				if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" {
					return ""
				}
				domain := parsed.Hostname()
				if matchedDomain != "" && matchedDomain != domain {
					return ""
				}
				matchedDomain = domain
			}
		}
	}
	if matchedDomain == "" {
		return ""
	}
	return matchedDomain
}
