package mockserver

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// https://github.com/aws-observability/aws-otel-test-framework/blob/terraform/mocked_servers/https/main.go
const (
	APP_SERVER_ADDR  = "http://127.0.0.1"
	APP_SERVER_PORT  = ":" + "8080"
	APP_SERVER       = APP_SERVER_ADDR + APP_SERVER_PORT
	DATA_SERVER_ADDR = "https://127.0.0.1"
	DATA_SERVER_PORT = ":" + "443"
	DATA_SERVER      = DATA_SERVER_ADDR + DATA_SERVER_PORT

	SEND_TEST_DATA_COUNT time.Duration = 10 * time.Second
	TPM_CHECK_INTERVAL   time.Duration = 10 * time.Second
)

func ResponseToString(res *http.Response) (string, error) {
	responseText, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(responseText), nil
}
func HttpsGetRequest(url string) (string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}

	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	return ResponseToString(res)
}
func HttpGetRequest(url string) (string, error) {
	res, err := http.Get(url)
	if err != nil {
		return "", err
	}
	return ResponseToString(res)

}
func HttpServerSanityCheck(t *testing.T, url string) {
	t.Helper()
	resString, err := HttpGetRequest(url)
	require.NoErrorf(t, err, "Healthcheck failed: %v", err)
	require.Contains(t, resString, HealthCheckMessage)
}
func HttpsServerSanityCheck(t *testing.T, url string) {
	t.Helper()
	resString, err := HttpsGetRequest(url)
	require.NoErrorf(t, err, "Healthcheck failed: %v", err)
	require.Contains(t, resString, HealthCheckMessage)
}
func HttpServerCheckData(t *testing.T) {
	t.Helper()
	resString, err := HttpGetRequest(APP_SERVER + "/check-data")
	require.NoErrorf(t, err, "Healthcheck failed: %v", err)
	// require.Contains(t, resString, HealthCheckMessage)
	t.Logf("resString: %s", resString)
}
func HttpServerCheckTPM(t *testing.T) {
	t.Helper()
	resString, err := HttpGetRequest(APP_SERVER + "/tpm")
	require.NoErrorf(t, err, "tpm failed: %v", err)
	var httpData map[string]interface{}
	json.Unmarshal([]byte(resString), &httpData)
	tpm, ok := httpData["tpm"]
	require.True(t, ok, "tpm json is broken")
	assert.Truef(t, tpm.(float64) > 1, "tpm is less than 1 %f", tpm)
}

func TestMockServer(t *testing.T) {
	serverControlChan := StartHttpServer()
	time.Sleep(3 * time.Second)
	HttpServerSanityCheck(t, APP_SERVER)
	HttpsServerSanityCheck(t, DATA_SERVER)
	time.Sleep(1 * time.Minute)
	serverControlChan <- 0

}

func TestStartMockServer(t *testing.T) {
	StartHttpServer()
}
