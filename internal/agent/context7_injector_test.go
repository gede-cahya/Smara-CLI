package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStepExecutor simulates a skill.StepExecutor for testing.
func mockStepExecutor(toolName string, args map[string]interface{}) (string, error) {
	switch toolName {
	case "resolve":
		lib, _ := args["libraryName"].(string)
		return "/" + lib + "/docs", nil
	case "get-library-documentation":
		uri, _ := args["uri"].(string)
		query, _ := args["query"].(string)
		return "Mock docs for " + uri + " (query: " + query + ")", nil
	default:
		return "", nil
	}
}

func TestContext7Injector_DetectAndInject(t *testing.T) {
	injector := NewContext7Injector()

	t.Run("injects docs for detected libraries", func(t *testing.T) {
		prompt := "buatkan komponen Next.js dengan app router"
		enriched, results, err := injector.DetectAndInject(prompt, mockStepExecutor)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "nextjs", results[0].Library)
		assert.Contains(t, enriched, "Mock docs for /next.js/docs")
		assert.Contains(t, enriched, "User request:")
	})

	t.Run("returns original prompt when no libraries detected", func(t *testing.T) {
		prompt := "halo apa kabar hari ini"
		enriched, results, err := injector.DetectAndInject(prompt, mockStepExecutor)
		require.NoError(t, err)
		assert.Empty(t, results)
		assert.Equal(t, prompt, enriched)
	})

	t.Run("caches resolved libraries in same injector", func(t *testing.T) {
		prompt := "deploy aplikasi golang menggunakan docker container"
		_, results, err := injector.DetectAndInject(prompt, mockStepExecutor)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		// Second call with same libraries should use cache
		prompt2 := "cara lain pakai golang dan docker"
		enriched2, results2, err := injector.DetectAndInject(prompt2, mockStepExecutor)
		require.NoError(t, err)
		// Both go and docker were already resolved in the first call
		assert.Len(t, results2, 2)
		assert.Contains(t, enriched2, "Mock docs for /go/docs")
		assert.Contains(t, enriched2, "Mock docs for /docker/docs")
	})

	t.Run("handles multiple library detections", func(t *testing.T) {
		fresh := NewContext7Injector()
		prompt := "buat komponen React pakai Tailwind dan deploy ke Vercel"
		enriched, results, err := fresh.DetectAndInject(prompt, mockStepExecutor)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
		assert.Contains(t, enriched, "User request:")
	})
}
