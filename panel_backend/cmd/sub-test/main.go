package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"panel_backend/internal/subscription"
	"strings"
	"time"
)

type IntegrationsResponse struct {
	Integrations []struct {
		ID              uint   `json:"id"`
		SubscriptionURL string `json:"subscriptionUrl"`
	} `json:"integrations"`
}

func main() {
	apiURL := strings.TrimSpace(os.Getenv("API_URL"))
	ciToken := strings.TrimSpace(os.Getenv("CI_TOKEN"))

	if apiURL == "" || ciToken == "" {
		fmt.Fprintln(os.Stderr, "API_URL and CI_TOKEN env vars required")
		os.Exit(1)
	}

	apiURL = strings.TrimRight(apiURL, "/")

	client := &http.Client{Timeout: 10 * time.Minute}

	all, err := fetchAll(client, apiURL, ciToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch integrations: %v\n", err)
		os.Exit(1)
	}

	if len(all.Integrations) == 0 {
		fmt.Println("no integrations to test")
		return
	}

	fmt.Printf("found %d integration(s)\n", len(all.Integrations))

	for _, integ := range all.Integrations {
		fmt.Printf("\ntesting integration %d: %s\n", integ.ID, integ.SubscriptionURL)
		testRunID := fmt.Sprintf("ci-%d-%d", integ.ID, time.Now().UnixMilli())

		if err := startTest(client, apiURL, ciToken, integ.ID, testRunID); err != nil {
			fmt.Fprintf(os.Stderr, "  start test failed: %v\n", err)
			continue
		}
		fmt.Printf("  started test run %s\n", testRunID)

		result, err := subscription.ImportSubscription(integ.SubscriptionURL)
		if err != nil {
			fmt.Printf("  test failed: %v\n", err)
			emptyResult := &subscription.ImportResult{TotalURLs: 0}
			rj, _ := json.Marshal(emptyResult)
			if err2 := completeTest(client, apiURL, ciToken, integ.ID, testRunID, string(rj), 0, 0, "failed", err.Error()); err2 != nil {
				fmt.Fprintf(os.Stderr, "  complete test (error) failed: %v\n", err2)
			}
			continue
		}

		rj, _ := json.Marshal(result)

		status := "completed"
		if result.FailCount == result.TotalURLs {
			status = "completed"
		}
		if err := completeTest(client, apiURL, ciToken, integ.ID, testRunID, string(rj), len(result.Working), result.TotalURLs, status, ""); err != nil {
			fmt.Fprintf(os.Stderr, "  complete test failed: %v\n", err)
			continue
		}

		fmt.Printf("  done: %d/%d working, %d failed\n", len(result.Working), result.TotalURLs, result.FailCount)
	}
}

func fetchAll(client *http.Client, apiURL, ciToken string) (*IntegrationsResponse, error) {
	req, err := http.NewRequest("GET", apiURL+"/api/integration/test/all", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-CI-Token", ciToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var ir IntegrationsResponse
	if err := json.Unmarshal(body, &ir); err != nil {
		return nil, err
	}
	return &ir, nil
}

func startTest(client *http.Client, apiURL, ciToken string, id uint, testRunID string) error {
	payload := fmt.Sprintf(`{"testRunId":"%s"}`, testRunID)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/integration/test/start/%d", apiURL, id), bytes.NewBufferString(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CI-Token", ciToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func completeTest(client *http.Client, apiURL, ciToken string, id uint, testRunID, resultJSON string, workingCount, totalCount int, status, errorMsg string) error {
	payloadMap := map[string]interface{}{
		"testRunId":    testRunID,
		"result":       resultJSON,
		"workingCount": workingCount,
		"totalCount":   totalCount,
		"status":       status,
	}
	if errorMsg != "" {
		payloadMap["errorMessage"] = errorMsg
	}
	payload, _ := json.Marshal(payloadMap)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/integration/test/complete/%d", apiURL, id), bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CI-Token", ciToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
