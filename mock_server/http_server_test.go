package mockserver

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	APP_SERVER_ADDR = "http://127.0.0.1"
	APP_SERVER_PORT = ":" + "8080"
	APP_SERVER      = APP_SERVER_ADDR + APP_SERVER_PORT
)

func HttpServerSanityCheck(t *testing.T) {
	t.Helper()
	res, err := http.Get(APP_SERVER)
	require.NoErrorf(t, err, "Healthcheck failed: %s", err.Error())
	require.Contains(t, res, HealthCheckMessage)
}

func TestHttpServer(t *testing.T) {
	go startHttpServer()
	time.Sleep(30 * time.Second)
	HttpServerSanityCheck(t)

}
