package scratch

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/session"
)

func mainTestSessionSQL() {
	dbPath := "test_session.db"
	defer os.Remove(dbPath)

	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sess := &session.Session{
		ID:         "test-id",
		Name:       "Test Session",
		State:      session.StateActive,
		Mode:       "ask",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		MCPServers: []string{"server1"},
	}

	fmt.Println("Attempting to create session...")
	err = store.CreateSession(sess)
	if err != nil {
		fmt.Printf("Expected error or success: %v\n", err)
	} else {
		fmt.Println("Session created successfully!")
	}
}
