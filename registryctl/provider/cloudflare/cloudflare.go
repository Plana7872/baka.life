package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/NyanLoli-Network/baka.life/registryctl/provider"
)

const endpoint = "https://api.cloudflare.com/client/v4"

type Client struct {
	Token      string
	ZoneID     string
	HTTPClient *http.Client
}

func NewFromEnv() (*Client, error) {
	client := &Client{
		Token:  os.Getenv("CLOUDFLARE_API_TOKEN"),
		ZoneID: os.Getenv("CLOUDFLARE_ZONE_ID"),
	}
	if client.Token == "" {
		return nil, fmt.Errorf("CLOUDFLARE_API_TOKEN is required")
	}
	if client.ZoneID == "" {
		return nil, fmt.Errorf("CLOUDFLARE_ZONE_ID is required")
	}
	return client, nil
}

func (c *Client) ListRecords(ctx context.Context) ([]provider.Record, error) {
	var all []provider.Record
	page := 1
	for {
		path := fmt.Sprintf("/zones/%s/dns_records?page=%d&per_page=100", url.PathEscape(c.ZoneID), page)
		var response responseEnvelope[[]dnsRecord]
		if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		for _, record := range response.Result {
			all = append(all, fromCloudflare(record))
		}
		if page >= response.ResultInfo.TotalPages || response.ResultInfo.TotalPages == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) CreateRecord(ctx context.Context, record provider.Record) (provider.Record, error) {
	var response responseEnvelope[dnsRecord]
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", url.PathEscape(c.ZoneID)), toCloudflare(record), &response); err != nil {
		return provider.Record{}, err
	}
	return fromCloudflare(response.Result), nil
}

func (c *Client) UpdateRecord(ctx context.Context, record provider.Record) (provider.Record, error) {
	if record.ID == "" {
		return provider.Record{}, fmt.Errorf("record ID is required for update")
	}
	path := fmt.Sprintf("/zones/%s/dns_records/%s", url.PathEscape(c.ZoneID), url.PathEscape(record.ID))
	var response responseEnvelope[dnsRecord]
	if err := c.do(ctx, http.MethodPut, path, toCloudflare(record), &response); err != nil {
		return provider.Record{}, err
	}
	return fromCloudflare(response.Result), nil
}

func (c *Client) DeleteRecord(ctx context.Context, record provider.Record) error {
	if record.ID == "" {
		return fmt.Errorf("record ID is required for delete")
	}
	path := fmt.Sprintf("/zones/%s/dns_records/%s", url.PathEscape(c.ZoneID), url.PathEscape(record.ID))
	var response responseEnvelope[struct{}]
	return c.do(ctx, http.MethodDelete, path, nil, &response)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare %s %s failed: %s: %s", method, path, resp.Status, string(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}

	status := responseStatus(out)
	if !status.success {
		return fmt.Errorf("cloudflare %s %s failed: %v", method, path, status.errors)
	}
	return nil
}

type responseEnvelope[T any] struct {
	Success    bool         `json:"success"`
	Errors     []apiMessage `json:"errors"`
	Messages   []apiMessage `json:"messages"`
	Result     T            `json:"result"`
	ResultInfo resultInfo   `json:"result_info"`
}

type apiMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type resultInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

type dnsRecord struct {
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Content  string         `json:"content,omitempty"`
	Priority *int           `json:"priority,omitempty"`
	Proxied  *bool          `json:"proxied,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type status struct {
	success bool
	errors  []apiMessage
}

func responseStatus(out any) status {
	data, _ := json.Marshal(out)
	var envelope struct {
		Success bool         `json:"success"`
		Errors  []apiMessage `json:"errors"`
	}
	_ = json.Unmarshal(data, &envelope)
	return status{success: envelope.Success, errors: envelope.Errors}
}

func fromCloudflare(record dnsRecord) provider.Record {
	return provider.Record{
		ID:       record.ID,
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		Priority: record.Priority,
		Proxied:  record.Proxied,
		Data:     record.Data,
	}
}

func toCloudflare(record provider.Record) dnsRecord {
	return dnsRecord{
		ID:       record.ID,
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		Priority: record.Priority,
		Proxied:  record.Proxied,
		Data:     record.Data,
	}
}
