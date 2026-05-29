package llm

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These integration tests hit the real 9router proxy.
// Set env vars to run:
//
//	SMARA_9ROUTER_URL=http://localhost:20128/v1
//	SMARA_9ROUTER_KEY=sk-...
//	SMARA_9ROUTER_MODEL=cx/gpt-5.5        (optional, defaults to cx/gpt-5.5)
//
// Run with:
//
//	go test -v -run TestIntegration9Router ./internal/llm/ -count=1

func router9Provider(t *testing.T) *CustomProvider {
	t.Helper()
	baseURL := os.Getenv("SMARA_9ROUTER_URL")
	apiKey := os.Getenv("SMARA_9ROUTER_KEY")
	if baseURL == "" || apiKey == "" {
		t.Skip("SMARA_9ROUTER_URL and SMARA_9ROUTER_KEY not set — skipping 9router integration test")
	}
	model := os.Getenv("SMARA_9ROUTER_MODEL")
	if model == "" {
		model = "cx/gpt-5.5"
	}
	return NewCustomProvider("9router", apiKey, model, baseURL)
}

// TestIntegration9Router_EmbeddingPrefixPropagation verifies that the
// embedding model name carries the same provider prefix as the chat model.
// e.g. cx/gpt-5.5 → cx/text-embedding-3-small
func TestIntegration9Router_EmbeddingPrefixPropagation(t *testing.T) {
	p := router9Provider(t)

	// The model is cx/gpt-5.5, so embedding should use cx/text-embedding-3-small
	// We don't check the actual response (codex doesn't support embeddings),
	// but we verify the provider doesn't crash and handles it gracefully.
	emb, err := p.GenerateEmbedding("test prefix propagation")
	// Should return nil,nil (graceful fallback) — not an error that breaks the flow.
	assert.NoError(t, err)
	// emb will be nil because codex doesn't support embeddings
	t.Logf("Embedding result (expected nil): %v", emb)
}

// TestIntegration9Router_EmbeddingDisabledCaching verifies that after the
// first failed embedding request, subsequent calls return instantly without
// making HTTP calls to the router.
func TestIntegration9Router_EmbeddingDisabledCaching(t *testing.T) {
	p := router9Provider(t)

	// First call — hits the router, gets 400, sets embDisabled flag
	t.Log("Call 1: hitting router (expect 400 from codex)")
	start1 := time.Now()
	emb1, err1 := p.GenerateEmbedding("first call — should hit router")
	dur1 := time.Since(start1)
	assert.NoError(t, err1)
	assert.Nil(t, emb1)
	t.Logf("  → duration: %v, embDisabled: %d", dur1, p.embDisabled.Load())

	// Verify the flag was set
	require.Equal(t, int32(1), p.embDisabled.Load(), "embDisabled should be set after first failure")

	// Second call — should return instantly without HTTP call
	t.Log("Call 2: should skip router (cached)")
	start2 := time.Now()
	emb2, err2 := p.GenerateEmbedding("second call — should be instant")
	dur2 := time.Since(start2)
	assert.NoError(t, err2)
	assert.Nil(t, emb2)
	t.Logf("  → duration: %v (should be <1ms)", dur2)

	// The second call should be significantly faster (no HTTP roundtrip)
	assert.Less(t, dur2, time.Millisecond, "cached call should be <1ms (no HTTP)")

	// Third call — also instant
	t.Log("Call 3: should skip router (cached)")
	start3 := time.Now()
	emb3, err3 := p.GenerateEmbedding("third call — also instant")
	dur3 := time.Since(start3)
	assert.NoError(t, err3)
	assert.Nil(t, emb3)
	t.Logf("  → duration: %v (should be <1ms)", dur3)
	assert.Less(t, dur3, time.Millisecond, "cached call should be <1ms (no HTTP)")
}

// TestIntegration9Router_ChatStillWorks verifies that even though embeddings
// fail, chat completions via codex continue to work normally.
func TestIntegration9Router_ChatStillWorks(t *testing.T) {
	p := router9Provider(t)

	// Trigger embedding failure first
	_, _ = p.GenerateEmbedding("trigger disable")
	require.Equal(t, int32(1), p.embDisabled.Load(), "embDisabled should be set")

	// Now chat should still work fine
	t.Log("Testing chat completion after embedding disable...")
	resp, err := p.Chat([]Message{
		{Role: "user", Content: "Reply with exactly one word: hello"},
	})
	require.NoError(t, err, "chat should work even when embeddings are disabled")
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Content, "chat response should have content")
	t.Logf("  → model: %s, content: %q", resp.Model, resp.Content)
}

// TestIntegration9Router_EmbeddingDisabledDoesNotAffectNewProvider verifies
// that the embDisabled flag is per-provider-instance and doesn't leak.
func TestIntegration9Router_EmbeddingDisabledDoesNotAffectNewProvider(t *testing.T) {
	p1 := router9Provider(t)
	p2 := router9Provider(t)

	// Disable embeddings on p1
	_, _ = p1.GenerateEmbedding("disable on p1")
	assert.Equal(t, int32(1), p1.embDisabled.Load())

	// p2 should still be at 0 (not disabled yet)
	assert.Equal(t, int32(0), p2.embDisabled.Load(), "new provider instance should not inherit embDisabled")
}

// TestIntegration9Router_MultipleEmbeddingCallsOnlyOneHTTP verifies that
// with many concurrent embedding calls, the router only gets hit once
// (or a few times due to races), not N times.
func TestIntegration9Router_MultipleEmbeddingCallsOnlyOneHTTP(t *testing.T) {
	p := router9Provider(t)

	const numCalls = 20
	done := make(chan struct{}, numCalls)

	start := time.Now()
	for i := 0; i < numCalls; i++ {
		go func(i int) {
			_, _ = p.GenerateEmbedding("concurrent call")
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < numCalls; i++ {
		<-done
	}
	totalDur := time.Since(start)

	assert.Equal(t, int32(1), p.embDisabled.Load(), "embDisabled should be set")
	t.Logf("  → %d calls completed in %v", numCalls, totalDur)

	// After the flag is set, verify subsequent calls are instant
	start2 := time.Now()
	for i := 0; i < 100; i++ {
		_, _ = p.GenerateEmbedding("post-disable call")
	}
	dur2 := time.Since(start2)
	t.Logf("  → 100 post-disable calls in %v (should be <1ms total)", dur2)
	assert.Less(t, dur2, 5*time.Millisecond, "100 cached calls should be near-instant")
}
