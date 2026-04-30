package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectWorkflowIntent_Triggers(t *testing.T) {
	assert.True(t, DetectWorkflowIntent("buatkan web SaaS restoran"))
	assert.True(t, DetectWorkflowIntent("bikin aplikasi e-commerce"))
	assert.True(t, DetectWorkflowIntent("create a mobile app"))
	assert.True(t, DetectWorkflowIntent("build project dashboard"))
	assert.True(t, DetectWorkflowIntent("generate website portfolio"))
	assert.True(t, DetectWorkflowIntent("buat platform learning"))
	assert.True(t, DetectWorkflowIntent("buatkan system payment"))
}

func TestDetectWorkflowIntent_NoTrigger(t *testing.T) {
	assert.False(t, DetectWorkflowIntent("hello"))
	assert.False(t, DetectWorkflowIntent("what is the weather"))
	assert.False(t, DetectWorkflowIntent("web")) // single keyword not enough
	assert.False(t, DetectWorkflowIntent("app")) // single keyword not enough
	assert.False(t, DetectWorkflowIntent(""))
	assert.False(t, DetectWorkflowIntent("buat makanan")) // only 1 match keyword
}

func TestDetectWorkflowIntent_EdgeCases(t *testing.T) {
	// Case insensitive
	assert.True(t, DetectWorkflowIntent("BUATKAN WEB APPS"))
	// Mixed case
	assert.True(t, DetectWorkflowIntent("Create Saas Platform"))
}
