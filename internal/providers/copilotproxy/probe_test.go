package copilotproxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBaseURLAddsV1(t *testing.T) {
	got, err := NormalizeBaseURL("http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:3000/v1" {
		t.Fatalf("unexpected normalized URL: %s", got)
	}
}

func TestProbeUsesModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := Probe(server.URL + "/v1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready {
		t.Fatalf("expected proxy probe to be ready: %+v", result)
	}
	if result.ProbePath != server.URL+"/v1/models" {
		t.Fatalf("unexpected probe path: %s", result.ProbePath)
	}
}

func TestProbeFallsBackToChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/models":
			response.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			if request.Method != http.MethodPost {
				response.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			response.WriteHeader(http.StatusUnauthorized)
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	result, err := Probe(server.URL + "/v1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready {
		t.Fatalf("expected fallback probe to be ready: %+v", result)
	}
	if result.ProbePath != server.URL+"/v1/chat/completions" {
		t.Fatalf("unexpected fallback probe path: %s", result.ProbePath)
	}
}

func TestProbeReportsVSCodeLMProxyWithoutV1Routes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/models":
			response.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			response.WriteHeader(http.StatusNotFound)
		case "/":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"status":"ok","message":"VSCode LM API Proxy server is running"}`))
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := Probe(server.URL + "/v1")
	if err == nil {
		t.Fatal("expected probe to explain missing /v1 routes")
	}
	want := fmt.Sprintf("VSCode LM API Proxy is running at %s/, but it does not expose the expected OpenAI-compatible /v1 endpoints yet", server.URL)
	if err.Error() != want {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}
