package services

import (
	"encoding/json"
	"errors"
	"net/url"
	"panel_backend/internal/models"
	"strings"

	"gorm.io/gorm"
)

const DefaultRealitySNI = "www.cloudflare.com"

type ProtocolSettings struct {
	RealitySNIs          []string `json:"realitySnis"`
	Hysteria2Masquerades []string `json:"hysteria2Masquerades"`
	DirectPackages       []string `json:"directPackages"`
	DirectDomains        []string `json:"directDomains"`
	ProxyDomains         []string `json:"proxyDomains"`
}

type ProtocolSettingsUpdateResponse struct {
	RealitySNIs          []string `json:"realitySnis"`
	Hysteria2Masquerades []string `json:"hysteria2Masquerades"`
	DirectPackages       []string `json:"directPackages"`
	DirectDomains        []string `json:"directDomains"`
	ProxyDomains         []string `json:"proxyDomains"`
	SyncedNodes          int      `json:"syncedNodes"`
	SyncError            string   `json:"syncError,omitempty"`
}

func defaultProtocolSettings() ProtocolSettings {
	return ProtocolSettings{
		RealitySNIs:          []string{DefaultRealitySNI},
		Hysteria2Masquerades: []string{},
		DirectPackages:       []string{},
		DirectDomains:        []string{},
		ProxyDomains:         []string{},
	}
}

func loadProtocolSettings(db *gorm.DB) (ProtocolSettings, error) {
	var admin models.AdminSetting
	err := db.First(&admin).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultProtocolSettings(), nil
		}
		return ProtocolSettings{}, err
	}

	return protocolSettingsFromAdmin(&admin)
}

func protocolSettingsFromAdmin(admin *models.AdminSetting) (ProtocolSettings, error) {
	settings := defaultProtocolSettings()
	if admin == nil {
		return settings, nil
	}

	realitySNIs, err := decodeStringList(admin.RealitySNIsJSON, settings.RealitySNIs)
	if err != nil {
		return ProtocolSettings{}, err
	}
	hysteria2Masquerades, err := decodeStringList(admin.Hysteria2ProxyJSON, settings.Hysteria2Masquerades)
	if err != nil {
		return ProtocolSettings{}, err
	}
	directPackages, err := decodeStringList(admin.DirectPackagesJSON, settings.DirectPackages)
	if err != nil {
		return ProtocolSettings{}, err
	}
	directDomains, err := decodeStringList(admin.DirectDomainsJSON, settings.DirectDomains)
	if err != nil {
		return ProtocolSettings{}, err
	}
	proxyDomains, err := decodeStringList(admin.ProxyDomainsJSON, settings.ProxyDomains)
	if err != nil {
		return ProtocolSettings{}, err
	}

	return normalizeProtocolSettings(ProtocolSettings{
		RealitySNIs:          realitySNIs,
		Hysteria2Masquerades: hysteria2Masquerades,
		DirectPackages:       directPackages,
		DirectDomains:        directDomains,
		ProxyDomains:         proxyDomains,
	})
}

func storeProtocolSettings(admin *models.AdminSetting, input ProtocolSettings) error {
	if admin == nil {
		return errors.New("admin settings are required")
	}

	settings, err := normalizeProtocolSettings(input)
	if err != nil {
		return err
	}

	realitySNIs, err := json.Marshal(settings.RealitySNIs)
	if err != nil {
		return err
	}
	hysteria2Masquerades, err := json.Marshal(settings.Hysteria2Masquerades)
	if err != nil {
		return err
	}
	directPackages, err := json.Marshal(settings.DirectPackages)
	if err != nil {
		return err
	}
	directDomains, err := json.Marshal(settings.DirectDomains)
	if err != nil {
		return err
	}
	proxyDomains, err := json.Marshal(settings.ProxyDomains)
	if err != nil {
		return err
	}

	admin.RealitySNIsJSON = string(realitySNIs)
	admin.Hysteria2ProxyJSON = string(hysteria2Masquerades)
	admin.DirectPackagesJSON = string(directPackages)
	admin.DirectDomainsJSON = string(directDomains)
	admin.ProxyDomainsJSON = string(proxyDomains)
	return nil
}

func normalizeProtocolSettings(input ProtocolSettings) (ProtocolSettings, error) {
	masquerades, err := normalizeMasqueradeList(input.Hysteria2Masquerades)
	if err != nil {
		return ProtocolSettings{}, err
	}
	directDomains := normalizeDomainSuffixList(input.DirectDomains)
	proxyDomains := normalizeDomainSuffixList(input.ProxyDomains)

	return ProtocolSettings{
		RealitySNIs:          normalizeStringList(input.RealitySNIs),
		Hysteria2Masquerades: masquerades,
		DirectPackages:       normalizeStringList(input.DirectPackages),
		DirectDomains:        directDomains,
		ProxyDomains:         proxyDomains,
	}, nil
}

func decodeStringList(raw string, fallback []string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), fallback...), nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}

	return values, nil
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))

	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized
}

func normalizeMasqueradeList(values []string) ([]string, error) {
	normalized := normalizeStringList(values)
	for index, value := range normalized {
		candidate := value
		if !strings.Contains(candidate, "://") {
			candidate = "https://" + candidate
		}

		parsed, err := url.ParseRequestURI(candidate)
		if err != nil {
			return nil, errors.New("hysteria2 reverse proxy targets must be valid hostnames or URLs")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, errors.New("hysteria2 reverse proxy targets must use http or https")
		}
		if parsed.Host == "" {
			return nil, errors.New("hysteria2 reverse proxy targets must include a host")
		}
		normalized[index] = parsed.String()
	}
	return normalized, nil
}

func normalizeDomainSuffixList(values []string) []string {
	normalized := normalizeStringList(values)
	for index, value := range normalized {
		normalized[index] = strings.TrimPrefix(strings.ToLower(value), ".")
	}
	return normalized
}
