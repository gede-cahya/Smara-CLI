package platform

import "sync"

// AuthManager manages user access control per platform.
// It supports whitelist (allow only specific users) and blacklist (block specific users).
// If AllowedUsers is empty, all users are allowed (unless blacklisted).
type AuthManager struct {
	mu       sync.RWMutex
	allowed  map[string]map[string]bool // platform → set of allowed user IDs
	blocked  map[string]map[string]bool // platform → set of blocked user IDs
}

// NewAuthManager creates a new AuthManager.
func NewAuthManager() *AuthManager {
	return &AuthManager{
		allowed: make(map[string]map[string]bool),
		blocked: make(map[string]map[string]bool),
	}
}

// SetAllowedUsers sets the whitelist for a platform.
// If the list is empty, all users are allowed (open access).
func (a *AuthManager) SetAllowedUsers(platform string, userIDs []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(userIDs) == 0 {
		delete(a.allowed, platform)
		return
	}
	m := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		m[id] = true
	}
	a.allowed[platform] = m
}

// SetBlockedUsers sets the blacklist for a platform.
func (a *AuthManager) SetBlockedUsers(platform string, userIDs []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(userIDs) == 0 {
		delete(a.blocked, platform)
		return
	}
	m := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		m[id] = true
	}
	a.blocked[platform] = m
}

// IsAllowed checks if a user is allowed to interact on a given platform.
// Logic: blocked → deny; whitelist exists and user not in it → deny; else → allow.
func (a *AuthManager) IsAllowed(platform, userID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check blacklist first
	if blocked, ok := a.blocked[platform]; ok {
		if blocked[userID] {
			return false
		}
	}

	// Check whitelist (if configured)
	if allowed, ok := a.allowed[platform]; ok {
		return allowed[userID]
	}

	// No whitelist = open access
	return true
}
