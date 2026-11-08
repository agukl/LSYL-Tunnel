//go:build windows

package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type ipAPIEntry struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	Query      string `json:"query"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	ISP        string `json:"isp"`
	Org        string `json:"org"`
	AS         string `json:"as"`
	ASName     string `json:"asname"`
	Hosting    bool   `json:"hosting"`
	Proxy      bool   `json:"proxy"`
	Mobile     bool   `json:"mobile"`
}

func isPublicIP(text string) bool {
	ip := net.ParseIP(strings.TrimSpace(text))
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func uniqueStrings(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func lookupIPAPIBatch(ctx context.Context, ips []string) (map[string]ipAPIEntry, error) {
	out := map[string]ipAPIEntry{}
	if len(ips) == 0 {
		return out, nil
	}
	client := &http.Client{Timeout: 5 * time.Second}
	const batchSize = 100
	for start := 0; start < len(ips); start += batchSize {
		end := start + batchSize
		if end > len(ips) {
			end = len(ips)
		}
		body, err := json.Marshal(ips[start:end])
		if err != nil {
			return out, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://ip-api.com/batch?fields=status,message,query,country,regionName,city,isp,org,as,asname,hosting,proxy,mobile&lang=zh-CN", bytes.NewReader(body))
		if err != nil {
			return out, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return out, err
		}
		func() {
			defer resp.Body.Close()
			var entries []ipAPIEntry
			if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
				out[""] = ipAPIEntry{Status: "fail", Message: err.Error()}
				return
			}
			for _, entry := range entries {
				out[entry.Query] = entry
			}
		}()
		if failure, ok := out[""]; ok {
			delete(out, "")
			return out, fmt.Errorf("归属地查询失败: %s", failure.Message)
		}
	}
	return out, nil
}

func attachAnalysisGeo(items []logAnalysisIPSource, geo map[string]ipAPIEntry) {
	for i := range items {
		entry, ok := geo[items[i].IP]
		if !ok || entry.Status != "success" {
			continue
		}
		items[i].Country = entry.Country
		items[i].Region = entry.RegionName
		items[i].City = entry.City
		items[i].ISP = entry.ISP
		items[i].Org = entry.Org
		items[i].AS = entry.AS
		items[i].ASName = entry.ASName
		items[i].Hosting = entry.Hosting
		items[i].Proxy = entry.Proxy
		items[i].Mobile = entry.Mobile
		items[i].Location = joinNonEmpty(" ", entry.Country, entry.RegionName, entry.City)
		items[i].Network = joinNonEmpty(" / ", entry.ISP, entry.Org)
		if items[i].Network == "" {
			items[i].Network = entry.ASName
		}
		items[i].NetworkType = analysisNetworkType(entry)
	}
}

func analysisNetworkType(entry ipAPIEntry) string {
	if entry.Proxy {
		return "代理"
	}
	if entry.Hosting {
		return "云主机"
	}
	if entry.Mobile {
		return "移动网络"
	}
	return "普通网络"
}

func joinNonEmpty(sep string, values ...string) string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return strings.Join(out, sep)
}
