// Package memory provides auto-compacting functionality to prevent context window overflow.
package memory

import (
	"fmt"
	"strings"
	"time"
)

// CompactionConfig configures memory compaction behavior.
type CompactionConfig struct {
	Enabled           bool          `json:"enabled"`
	MaxTotalMemories  int           `json:"max_total_memories"`
	MaxAgeDays        int           `json:"max_age_days"`
	MinRelevanceScore float64       `json:"min_relevance_score"`
	CompactInterval   time.Duration `json:"compact_interval"`
}

// DefaultCompactionConfig provides sensible defaults.
var DefaultCompactionConfig = CompactionConfig{
	Enabled:           true,
	MaxTotalMemories:  5000,
	MaxAgeDays:        90,
	MinRelevanceScore: 0.1,
	CompactInterval:   30 * time.Minute,
}

// Compactor manages automatic memory compaction.
type Compactor struct {
	config CompactionConfig
	store  MemoryStore
	stats  CompactionStats
}

// CompactionStats tracks compaction activity.
type CompactionStats struct {
	TotalCompactions int       `json:"total_compactions"`
	MemoriesRemoved  int       `json:"memories_removed"`
	MemoriesMerged   int       `json:"memories_merged"`
	LastCompaction   time.Time `json:"last_compaction"`
	SpaceSaved       int64     `json:"space_saved_bytes"`
}

// NewCompactor creates a new memory compactor.
func NewCompactor(store MemoryStore, config CompactionConfig) *Compactor {
	return &Compactor{
		config: config,
		store:  store,
		stats:  CompactionStats{},
	}
}

// Compact performs a compaction pass on old and low-relevance memories.
func (c *Compactor) Compact() error {
	if !c.config.Enabled {
		return nil
	}

	// 1. Get memory count
	memories, err := c.store.List(0, c.config.MaxTotalMemories+1)
	if err != nil {
		return fmt.Errorf("gagal list memories: %w", err)
	}

	if len(memories) < c.config.MaxTotalMemories {
		// Under limit, only age-based cleanup
		return c.compactByAge(memories)
	}

	// 2. Over limit — remove oldest and least relevant
	if err := c.compactByAge(memories); err != nil {
		return err
	}

	c.stats.LastCompaction = time.Now()
	c.stats.TotalCompactions++
	return nil
}

// compactByAge removes memories older than MaxAgeDays.
func (c *Compactor) compactByAge(memories []Memory) error {
	if c.config.MaxAgeDays <= 0 {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -c.config.MaxAgeDays)
	removed := 0

	for _, m := range memories {
		if m.CreatedAt.Before(cutoff) {
			// Archive old memory instead of deleting
			if err := c.store.ArchiveMemory(m.ID); err == nil {
				removed++
			}
		}
	}

	c.stats.MemoriesRemoved += removed
	return nil
}

// CompactWorkspace compacts memories for a specific workspace.
func (c *Compactor) CompactWorkspace(workspaceID int64) error {
	if !c.config.Enabled {
		return nil
	}

	memories, err := c.store.List(workspaceID, c.config.MaxTotalMemories+1)
	if err != nil {
		return err
	}

	return c.compactByAge(memories)
}

// GetStats returns compaction statistics.
func (c *Compactor) GetStats() CompactionStats {
	return c.stats
}

// UpdateConfig updates the compaction configuration.
func (c *Compactor) UpdateConfig(config CompactionConfig) {
	c.config = config
}

// SummarizeMemories creates a summary of related memories to reduce count.
func SummarizeMemories(memories []Memory) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Summary of %d memories]\n", len(memories)))

	// Group by source
	bySource := make(map[string][]Memory)
	for _, m := range memories {
		bySource[m.Source] = append(bySource[m.Source], m)
	}

	for source, items := range bySource {
		sb.WriteString(fmt.Sprintf("From %s (%d items): ", source, len(items)))
		for i, item := range items {
			if i > 2 {
				sb.WriteString("...")
				break
			}
			summary := item.Content
			if len(summary) > 80 {
				summary = summary[:80] + "..."
			}
			sb.WriteString(summary)
			if i < len(items)-1 && i < 2 {
				sb.WriteString(" | ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ShouldCompact checks if compaction should run based on memory count.
func (c *Compactor) ShouldCompact() bool {
	if !c.config.Enabled {
		return false
	}

	// Check interval
	if time.Since(c.stats.LastCompaction) < c.config.CompactInterval {
		return false
	}

	memories, err := c.store.List(0, c.config.MaxTotalMemories+1)
	if err != nil {
		return false
	}

	return len(memories) >= c.config.MaxTotalMemories
}

// AutoCompact runs compaction if conditions are met.
func (c *Compactor) AutoCompact() error {
	if !c.ShouldCompact() {
		return nil
	}
	return c.Compact()
}
