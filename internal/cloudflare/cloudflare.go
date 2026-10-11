// Package cloudflare 提供 Cloudflare API 客户端与公网 IP 检测。
package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiBase   = "https://api.cloudflare.com/client/v4"
	userAgent = "ddns-go/0.1"
)

// Client 是 Cloudflare API 客户端。
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// NewClient 创建一个 Cloudflare API 客户端。
func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: apiBase,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// cfResponse 是 Cloudflare API 统一响应外壳。
type cfResponse struct {
	Success  bool            `json:"success"`
	Errors   []cfAPIError    `json:"errors"`
	Messages []json.RawMessage `json:"messages"`
	Result   json.RawMessage `json:"result"`
	ResultInfo struct {
		Count int `json:"count"`
	} `json:"result_info"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// do 执行一次 API 请求，解析统一响应。
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*cfResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s %s 失败: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s 返回 HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var cf cfResponse
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !cf.Success {
		return nil, fmt.Errorf("Cloudflare 返回错误: %v", cf.Errors)
	}
	return &cf, nil
}

// DetectPublicIP 从给定的 IP 源列表探测公网 IPv4，逐个尝试，返回第一个合法 IP。
func DetectPublicIP(ctx context.Context, sources []string) (string, error) {
	probe := &http.Client{Timeout: 5 * time.Second}
	defer probe.CloseIdleConnections()

	var lastErr error
	for _, source := range sources {
		src := strings.TrimSpace(source)
		if src == "" {
			continue
		}
		ip, err := fetchIP(ctx, probe, src)
		if err == nil && ip != "" {
			return ip, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("IP 源列表为空")
	}
	return "", fmt.Errorf("所有 IP 源均无法获取公网 IP: %w", lastErr)
}

// fetchIP 从单个源获取并校验 IPv4。
func fetchIP(ctx context.Context, client *http.Client, source string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	ip = strings.Trim(ip, `"'`)
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return "", fmt.Errorf("%s 返回的 %q 不是合法 IPv4", source, ip)
	}
	return parsed.To4().String(), nil
}

// ---------------------------------------------------------------------------
// 区域 (Zone) 与 DNS 记录操作
// ---------------------------------------------------------------------------

type zoneResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ZoneID 根据区域名查询对应的 zone_id。
func (c *Client) ZoneID(ctx context.Context, zoneName string) (string, error) {
	path := "/zones?name=" + url.QueryEscape(zoneName)
	cf, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	var zones []zoneResult
	if len(cf.Result) > 0 {
		if err := json.Unmarshal(cf.Result, &zones); err != nil {
			return "", fmt.Errorf("解析 zones 结果失败: %w", err)
		}
	}
	for _, z := range zones {
		if strings.EqualFold(z.Name, zoneName) {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("未找到区域 %s", zoneName)
}

// DNSRecord 表示一条 Cloudflare DNS 记录。
type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

// FindARecord 查询 type=A 且 name 匹配的 DNS 记录。
// 返回记录指针；若不存在返回 (nil, nil)。
func (c *Client) FindARecord(ctx context.Context, zoneID, name string) (*DNSRecord, error) {
	path := fmt.Sprintf("/zones/%s/dns_records?type=A&name=%s", zoneID, url.QueryEscape(name))
	cf, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if len(cf.Result) > 0 {
		if err := json.Unmarshal(cf.Result, &records); err != nil {
			return nil, fmt.Errorf("解析 dns_records 结果失败: %w", err)
		}
	}
	for i := range records {
		if records[i].Type == "A" && strings.EqualFold(records[i].Name, name) {
			return &records[i], nil
		}
	}
	return nil, nil
}

type dnsWritePayload struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// CreateARecord 创建一条 A 记录。
func (c *Client) CreateARecord(ctx context.Context, zoneID, name, content string, ttl int) error {
	if ttl == 0 {
		ttl = 1
	}
	payload := dnsWritePayload{Type: "A", Name: name, Content: content, TTL: ttl, Proxied: false}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), strings.NewReader(string(body)))
	return err
}

// UpdateARecord 更新现有 A 记录的 content。
func (c *Client) UpdateARecord(ctx context.Context, zoneID, recordID, name, content string, ttl int) error {
	if ttl == 0 {
		ttl = 1
	}
	payload := dnsWritePayload{Type: "A", Name: name, Content: content, TTL: ttl, Proxied: false}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPatch, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID), strings.NewReader(string(body)))
	return err
}
