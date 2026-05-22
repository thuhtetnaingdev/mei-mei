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

type appendTestPayload struct {
	TestRunID    string           `json:"testRunId"`
	Tested       []map[string]any `json:"tested"`
	Working      []map[string]any `json:"working"`
	WorkingCount int              `json:"workingCount"`
	TotalCount   int              `json:"totalCount"`
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

		uris, err := subscription.FetchSubscription(integ.SubscriptionURL)
		if err != nil {
			fmt.Printf("  fetch failed: %v\n", err)
			emptyResult := &subscription.ImportResult{TotalURLs: 0}
			rj, _ := json.Marshal(emptyResult)
			if err2 := completeTest(client, apiURL, ciToken, integ.ID, testRunID, string(rj), 0, 0, "failed", err.Error()); err2 != nil {
				fmt.Fprintf(os.Stderr, "  complete test (error) failed: %v\n", err2)
			}
			continue
		}
		parsed := subscription.ParseAll(uris)
		fmt.Printf("  parsed %d proxies\n", len(parsed))

		var allTested []subscription.TestResult
		var allWorking []subscription.SingboxOutbound

		for i := 0; i < len(parsed); i += 1000 {
			end := i + 1000
			if end > len(parsed) {
				end = len(parsed)
			}

			batch := parsed[i:end]
			fmt.Printf("  testing batch %d/%d (%d proxies)\n", i/1000+1, (len(parsed)+999)/1000, len(batch))

			tested := subscription.TestAllWithConcurrency(batch, 1000)
			working := subscription.ConvertWorking(batch, tested)

			allTested = append(allTested, tested...)
			allWorking = append(allWorking, working...)

			testedAny := make([]map[string]any, len(tested))
			for j, t := range tested {
				testedAny[j] = testResultToAny(t)
			}
			workingAny := make([]map[string]any, len(working))
			for j, w := range working {
				workingAny[j] = singboxOutboundToAny(w)
			}

			if err := appendTest(client, apiURL, ciToken, integ.ID, testRunID,
				testedAny, workingAny, len(allWorking), len(uris)); err != nil {
				fmt.Fprintf(os.Stderr, "  append batch %d failed: %v\n", i/1000, err)
			}
		}

		result := &subscription.ImportResult{
			Parsed:    parsed,
			Tested:    allTested,
			Working:   allWorking,
			FailCount: len(allTested) - len(allWorking),
			TotalURLs: len(uris),
		}
		rj, _ := json.Marshal(result)

		status := "completed"
		if result.FailCount == result.TotalURLs {
			status = "completed"
		}
		if err := completeTest(client, apiURL, ciToken, integ.ID, testRunID, string(rj), len(allWorking), result.TotalURLs, status, ""); err != nil {
			fmt.Fprintf(os.Stderr, "  complete test failed: %v\n", err)
			continue
		}

		fmt.Printf("  done: %d/%d working, %d failed\n", len(allWorking), result.TotalURLs, result.FailCount)
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

func appendTest(client *http.Client, apiURL, ciToken string, id uint, testRunID string, tested, working []map[string]any, workingCount, totalCount int) error {
	payload := appendTestPayload{
		TestRunID:    testRunID,
		Tested:       tested,
		Working:      working,
		WorkingCount: workingCount,
		TotalCount:   totalCount,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/integration/test/append/%d", apiURL, id), bytes.NewBuffer(body))
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
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func testResultToAny(tr subscription.TestResult) map[string]any {
	return map[string]any{
		"uri":       tr.URI,
		"protocol":  tr.Protocol,
		"host":      tr.Host,
		"port":      tr.Port,
		"working":   tr.Working,
		"latencyMs": tr.LatencyMs,
		"speedMbps": tr.SpeedMbps,
		"error":     tr.Error,
	}
}

func singboxOutboundToAny(so subscription.SingboxOutbound) map[string]any {
	return map[string]any{
		"tag":       so.Tag,
		"config":    so.Config,
		"latencyMs": so.LatencyMs,
		"speedMbps": so.SpeedMbps,
		"remark":    so.Remark,
		"rawUri":    so.RawURI,
	}
}
