package aiprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proxyHelperEnv names the provider the helper process calls; set only in the
// child the proxy tests start.
const proxyHelperEnv = "AIPROVIDER_PROXY_HELPER"

// stubHost is a provider address no resolver answers (.test is reserved), so a
// request that skips the proxy fails instead of reaching anything.
const stubHost = "provider.stub.test"

// TestProxyHelper is the child process: it calls one provider at stubHost and
// exits. It runs only when the parent sets proxyHelperEnv, because the proxy
// variables are read once per process and must be set before the first
// request.
func TestProxyHelper(t *testing.T) {
	id := os.Getenv(proxyHelperEnv)
	if id == "" {
		t.Skip("run by the proxy tests")
	}
	scheme := os.Getenv(proxyHelperEnv + "_SCHEME")
	if scheme == "" {
		scheme = "http"
	}
	p, err := NewProvider(ProviderID(id), Config{BaseURL: scheme + "://" + stubHost, Model: "m", APIKey: "k"})
	require.NoError(t, err)
	_, _ = p.Chat(context.Background(), []Message{TextMessage(RoleUser, "hi")})
}

// recordingProxy is a forward proxy that records the host of every request it
// is asked to forward and answers with reply itself, so nothing leaves the
// machine.
func recordingProxy(t *testing.T, reply string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var hosts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hosts = append(hosts, r.Host)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), hosts...)
	}
}

func runProxyHelper(t *testing.T, id ProviderID, env ...string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProxyHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), append([]string{proxyHelperEnv + "=" + string(id)}, env...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// A person behind a proxy reaches every provider through it: each provider's
// client honours HTTP_PROXY, HTTPS_PROXY and NO_PROXY as Go's default
// transport does.
func TestEveryProviderHonoursTheProxyEnvironment(t *testing.T) {
	cases := []struct {
		id    ProviderID
		reply string
	}{
		{Anthropic, anthropicReply},
		{OpenAI, openaiReply},
		{Gemini, geminiReply},
		{AzureOpenAI, openaiReply},
		{Ollama, `{"message":{"content":"ok"},"done":true}`},
	}
	for _, c := range cases {
		t.Run(string(c.id), func(t *testing.T) {
			proxy, seen := recordingProxy(t, c.reply)
			runProxyHelper(t, c.id, "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL, "NO_PROXY=", "http_proxy=", "https_proxy=", "no_proxy=")
			assert.Contains(t, seen(), stubHost, "the request did not go through the proxy")
		})
		t.Run(string(c.id)+" with NO_PROXY", func(t *testing.T) {
			proxy, seen := recordingProxy(t, c.reply)
			runProxyHelper(t, c.id, "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL, "NO_PROXY="+stubHost, "http_proxy=", "https_proxy=", "no_proxy=")
			assert.Empty(t, seen(), "a host NO_PROXY names went through the proxy")
		})
	}
}

// An https provider is reached through HTTPS_PROXY with a CONNECT to its host.
func TestHTTPSProviderTunnelsThroughTheProxy(t *testing.T) {
	proxy, seen := recordingProxy(t, "")
	runProxyHelper(t, Anthropic, proxyHelperEnv+"_SCHEME=https",
		"HTTPS_PROXY="+proxy.URL, "HTTP_PROXY=", "NO_PROXY=", "http_proxy=", "https_proxy=", "no_proxy=")
	assert.Contains(t, seen(), stubHost+":443")
}
