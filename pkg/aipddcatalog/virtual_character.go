package aipddcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const VirtualCharacterCatalogPath = "/v1/new-api/virtual-character-catalog"

var ErrVirtualCharacterCatalogNotModified = errors.New("AIPDD virtual character catalog not modified")

// defaultCatalogClient bounds callers that pass a context without a deadline,
// such as the admin-triggered sync running on a Gin request context.
var defaultCatalogClient = &http.Client{Timeout: 60 * time.Second}

type VirtualCharacterCatalog struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Revision      string                        `json:"revision"`
	Version       string                        `json:"version"`
	PublishedAt   string                        `json:"publishedAt"`
	Items         []VirtualCharacterCatalogItem `json:"items"`
}

type VirtualCharacterCatalogItem struct {
	Provider        string   `json:"provider"`
	ProviderAssetID string   `json:"provider_asset_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	CoverURL        string   `json:"cover_url"`
	Nationality     string   `json:"nationality"`
	Gender          string   `json:"gender"`
	AgeMin          *int     `json:"age_min"`
	AgeMax          *int     `json:"age_max"`
	Occupation      string   `json:"occupation"`
	Temperament     string   `json:"temperament"`
	Enabled         bool     `json:"enabled"`
	SortOrder       int      `json:"sort_order"`
}

type virtualCharacterCatalogResponse struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Data    VirtualCharacterCatalog `json:"data"`
}

// FetchVirtualCharacterCatalog loads the published official catalog from AIPDD.
// Pass ifNoneMatch (revision hash or full ETag) to allow HTTP 304.
func FetchVirtualCharacterCatalog(ctx context.Context, client *http.Client, baseURL, apiKey, ifNoneMatch string) (VirtualCharacterCatalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = defaultCatalogClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeBaseURL(baseURL)+VirtualCharacterCatalogPath, nil)
	if err != nil {
		return VirtualCharacterCatalog{}, err
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("X-API-Key", key)
	}
	if etag := strings.TrimSpace(ifNoneMatch); etag != "" {
		if !strings.HasPrefix(etag, "W/") && !strings.HasPrefix(etag, "\"") {
			etag = `W/"` + etag + `"`
		}
		request.Header.Set("If-None-Match", etag)
		query := request.URL.Query()
		query.Set("revision", strings.Trim(strings.TrimPrefix(strings.TrimSpace(ifNoneMatch), "W/"), `"`))
		request.URL.RawQuery = query.Encode()
	}
	response, err := client.Do(request)
	if err != nil {
		return VirtualCharacterCatalog{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return VirtualCharacterCatalog{}, ErrVirtualCharacterCatalogNotModified
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return VirtualCharacterCatalog{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return VirtualCharacterCatalog{}, fmt.Errorf("AIPDD virtual character catalog returned HTTP %d", response.StatusCode)
	}
	var envelope virtualCharacterCatalogResponse
	if err := common.Unmarshal(body, &envelope); err != nil {
		return VirtualCharacterCatalog{}, fmt.Errorf("decode AIPDD virtual character catalog: %w", err)
	}
	if err := validateAIPDDResponse(envelope.Code, envelope.Message, "fetch AIPDD virtual character catalog"); err != nil {
		return VirtualCharacterCatalog{}, err
	}
	if err := envelope.Data.Validate(); err != nil {
		return VirtualCharacterCatalog{}, err
	}
	return envelope.Data, nil
}

func (catalog VirtualCharacterCatalog) Validate() error {
	if catalog.SchemaVersion != 0 && catalog.SchemaVersion != 1 {
		return fmt.Errorf("unsupported virtual character catalog schemaVersion %d", catalog.SchemaVersion)
	}
	if strings.TrimSpace(catalog.Revision) == "" {
		return errors.New("virtual character catalog revision is required")
	}
	if strings.TrimSpace(catalog.Version) == "" {
		return errors.New("virtual character catalog version is required")
	}
	if len(catalog.Items) == 0 {
		return errors.New("virtual character catalog items are empty")
	}
	if len(catalog.Items) > 10000 {
		return errors.New("virtual character catalog exceeds 10000 items")
	}
	seen := make(map[string]struct{}, len(catalog.Items))
	for i := range catalog.Items {
		item := &catalog.Items[i]
		assetID := strings.TrimPrefix(strings.TrimSpace(item.ProviderAssetID), "asset://")
		if assetID == "" || strings.ContainsAny(assetID, " \t\r\n") {
			return fmt.Errorf("catalog item %d has an invalid provider_asset_id", i)
		}
		if _, exists := seen[assetID]; exists {
			return fmt.Errorf("catalog contains duplicate provider_asset_id %s", assetID)
		}
		seen[assetID] = struct{}{}
		item.ProviderAssetID = assetID
		if strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("catalog item %s is missing name", assetID)
		}
		cover := strings.TrimSpace(item.CoverURL)
		if cover == "" || (!strings.HasPrefix(cover, "https://") && !strings.HasPrefix(cover, "http://")) {
			return fmt.Errorf("catalog item %s has an invalid cover_url", assetID)
		}
		if strings.Contains(cover, "X-Tos-Signature=") || strings.Contains(cover, "/sign") {
			return fmt.Errorf("catalog item %s cover_url must be a permanent URL", assetID)
		}
		item.CoverURL = cover
		if strings.TrimSpace(item.Provider) == "" {
			item.Provider = "volc"
		}
	}
	return nil
}
