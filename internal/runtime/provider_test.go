package runtime

import (
	"net/http"
	"testing"
)

func TestBuildUpstreamURLKeepsSingleV1ForProxyBase(t *testing.T) {
	target := upstreamTarget{
		BaseURL: "http://localhost:4000/openai/v1",
		Headers: make(http.Header),
	}

	got := buildUpstreamURL(target, "/v1/models")
	want := "http://localhost:4000/openai/v1/models"
	if got != want {
		t.Fatalf("unexpected upstream URL: got %s want %s", got, want)
	}
}

func TestBuildUpstreamURLKeepsV1ForNativeBase(t *testing.T) {
	target := upstreamTarget{
		BaseURL: "https://api.individual.githubcopilot.com",
		Headers: make(http.Header),
	}

	got := buildUpstreamURL(target, "/v1/chat/completions")
	want := "https://api.individual.githubcopilot.com/chat/completions"
	if got != want {
		t.Fatalf("unexpected upstream URL: got %s want %s", got, want)
	}
}
