package agent

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLToText_StripsScriptsAndTags(t *testing.T) {
	html := `<html>
<head><title>T</title><style>body{color:red}</style></head>
<body>
  <nav>menu</nav>
  <script>alert('x')</script>
  <h1>Hello</h1>
  <p>World<br>!</p>
</body></html>`
	out := htmlToText(html)
	assert.NotContains(t, out, "<script>")
	assert.NotContains(t, out, "alert")
	assert.NotContains(t, out, "<h1>")
	assert.NotContains(t, out, "menu")
	assert.Contains(t, out, "Hello")
	assert.Contains(t, out, "World")
}

func TestDecodeEntities(t *testing.T) {
	assert.Equal(t, "Tom & Jerry", decodeEntities("Tom &amp; Jerry"))
	assert.Equal(t, "\"quoted\"", decodeEntities("&quot;quoted&quot;"))
	assert.Equal(t, "©", decodeEntities("&#169;"))
}

func TestExportData_CSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	_, err := exportData(map[string]interface{}{
		"format": "csv",
		"data": []interface{}{
			map[string]interface{}{"nama": "Budi", "partai": "X", "umur": float64(45)},
			map[string]interface{}{"nama": "Ani", "partai": "Y", "umur": float64(38)},
		},
		"columns": []interface{}{"nama", "partai", "umur"},
		"path":    path,
	})
	require.NoError(t, err)

	// Read back and assert
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"nama", "partai", "umur"}, rows[0])
	assert.Equal(t, []string{"Budi", "X", "45"}, rows[1])
	assert.Equal(t, []string{"Ani", "Y", "38"}, rows[2])
}

func TestExportData_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	_, err := exportData(map[string]interface{}{
		"format": "json",
		"data": []interface{}{
			map[string]interface{}{"a": "1", "b": "2"},
		},
		"path": path,
	})
	require.NoError(t, err)

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed []map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "1", parsed[0]["a"])
}

func TestExportData_Markdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	_, err := exportData(map[string]interface{}{
		"format": "md",
		"title":  "Tabel Pejabat",
		"data": []interface{}{
			map[string]interface{}{"nama": "Budi", "jabatan": "Menteri X"},
			map[string]interface{}{"nama": "Ani | dengan pipe", "jabatan": "Dirut"},
		},
		"columns": []interface{}{"nama", "jabatan"},
		"path":    path,
	})
	require.NoError(t, err)

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(b)
	assert.Contains(t, content, "# Tabel Pejabat")
	assert.Contains(t, content, "| nama | jabatan |")
	// Pipe in cell should be escaped
	assert.Contains(t, content, "Ani \\| dengan pipe")
}

func TestExportData_InferColumnsFromFirstRow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	_, err := exportData(map[string]interface{}{
		"format": "csv",
		"data": []interface{}{
			map[string]interface{}{"name": "a"},
			map[string]interface{}{"name": "b"},
		},
		"path": path,
	})
	require.NoError(t, err)

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(b), "name")
}

func TestExportData_RejectsInvalidFormat(t *testing.T) {
	_, err := exportData(map[string]interface{}{
		"format": "docx",
		"data":   []interface{}{map[string]interface{}{"x": "1"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format harus")
}

func TestExportData_RejectsEmptyData(t *testing.T) {
	_, err := exportData(map[string]interface{}{
		"format": "csv",
		"data":   []interface{}{},
	})
	require.Error(t, err)
}

func TestValueToString_DropsIntegerDecimal(t *testing.T) {
	assert.Equal(t, "45", valueToString(float64(45)))
	assert.Equal(t, "3.14", valueToString(3.14))
	assert.Equal(t, "true", valueToString(true))
	assert.Equal(t, "", valueToString(nil))
}

func TestFetchWebPage_RejectsInvalidURL(t *testing.T) {
	_, err := fetchWebPage("not a url", 1000)
	require.Error(t, err)
	_, err = fetchWebPage("ftp://x", 1000)
	require.Error(t, err)
}

func TestBuiltinTools_IncludesWebAndExport(t *testing.T) {
	tools := GetBuiltinTools()
	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name] = true
	}
	assert.True(t, names["web_search"], "web_search should remain")
	assert.True(t, names["web_fetch"], "web_fetch should be registered")
	assert.True(t, names["export_data"], "export_data should be registered")
}

// TestCapText_TruncatesAndAnnotates verifies the header + size limit.
func TestCapText_TruncatesAndAnnotates(t *testing.T) {
	body := strings.Repeat("x", 100)
	out := capText(body, 20, "https://example.com", "text/html")
	assert.Contains(t, out, "Fetched: https://example.com")
	assert.Contains(t, out, "dipotong")
	// Headers + truncated body
	assert.Contains(t, out, strings.Repeat("x", 20))
}


func TestLooksLikeChallenge(t *testing.T) {
	cases := map[string]bool{
		"<html><body><h1>Just a moment...</h1></body></html>":                               true,
		"<script>cf_chl_opt={}</script>":                                                   true,
		"<div id='cf-browser-verification'>Checking your browser before accessing</div>":  true,
		"<title>Checking your browser</title>":                                              true,
		"<html><body><h1>Article headline</h1><p>normal content here</p></body></html>":    false,
		"":                                                                                 false,
	}
	for html, want := range cases {
		assert.Equal(t, want, looksLikeChallenge(html), "html=%q", html[:min(50, len(html))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestIsBlockingError(t *testing.T) {
	cases := map[string]bool{
		"HTTP 403 dari https://x.com": true,
		"HTTP 503 Service Unavailable": true,
		"gagal connect: Cloudflare blocked": true,
		"Forbidden": true,
		"timeout exceeded": false,
		"dns lookup failed": false,
		"": false,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = fmt.Errorf("%s", msg)
		}
		assert.Equal(t, want, isBlockingError(err), "msg=%q", msg)
	}
}
