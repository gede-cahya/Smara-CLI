package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/graphify"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
	"github.com/stretchr/testify/require"
)

func TestGraphDataRouteIsRegistered(t *testing.T) {
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	graphStore, err := graphify.NewGraphStore(store.DB())
	require.NoError(t, err)

	graph := graphify.NewGraph("test_graph", t.TempDir())
	graph.AddNode(&graphify.Node{ID: "node-1", Label: "Node 1", Type: "function"})
	require.NoError(t, graphStore.SaveGraph(graph))

	mux := http.NewServeMux()
	(&Server{MemStore: store}).registerGraphRoutes(mux)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/graph/data?id=test_graph", nil)
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"node-1"`)
}
