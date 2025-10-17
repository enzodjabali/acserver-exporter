package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func FetchHTTPInfo(m *ACServerMonitor) error {
	url := fmt.Sprintf("http://%s:%d/INFO", m.httpHost, m.httpPort)
	
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP API request failed: %v", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	
	var info ServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}
	
	m.mu.Lock()
	m.serverInfo = &info
	m.mu.Unlock()
	
	return nil
}
