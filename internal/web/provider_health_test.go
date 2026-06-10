package web

import (
	"net"
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProviderHealthEndpointUsesConfiguredProvider(t *testing.T) {
	cfg := &config.SmaraConfig{Provider: "custom", CustomBaseURL: "http://localhost:20128/v1"}
	require.Equal(t, "http://localhost:20128/v1", providerHealthEndpoint(cfg))
}

func TestProviderReachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	online, message := providerReachable("http://" + listener.Addr().String() + "/v1")
	require.True(t, online)
	require.Empty(t, message)
}

func TestProviderReachableRejectsInvalidEndpoint(t *testing.T) {
	online, message := providerReachable("not-a-url")
	require.False(t, online)
	require.NotEmpty(t, message)
}
