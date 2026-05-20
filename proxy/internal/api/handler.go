package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type UserMap map[string]string

type Handler struct {
	proxyURL string
	mapToken string
}

func New(proxyURL, mapToken string) *Handler {
	return &Handler{proxyURL: strings.TrimRight(proxyURL, "/"), mapToken: mapToken}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/sub/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	username := strings.TrimPrefix(r.URL.Path, "/sub/")
	username = strings.TrimSuffix(username, "/")
	if username == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}

	userMap, err := h.fetchUserMap()
	if err != nil {
		log.Printf("failed to fetch user map: %v", err)
		http.Error(w, "failed to fetch user map", http.StatusBadGateway)
		return
	}

	uuid, ok := userMap[username]
	if !ok {
		http.Error(w, fmt.Sprintf("user %q not found in map", username), http.StatusNotFound)
		return
	}

	target := h.proxyURL + "/profiles/singbox/" + uuid
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(r.Method, target, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxyReq.Header = r.Header.Clone()

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		log.Printf("proxy request failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) fetchUserMap() (UserMap, error) {
	target := h.proxyURL + "/api/public/migration-map"
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.mapToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("map API returned status %d", resp.StatusCode)
	}

	var m UserMap
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}
