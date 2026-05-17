package workflow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/memory"
)

func isMemoryNode(a CustomAgent) bool {
	if a.Memory != nil {
		return true
	}
	role := strings.ToLower(a.Role)
	if strings.HasPrefix(role, "memory") || strings.Contains(role, "memory-") {
		return true
	}
	for _, sk := range a.Skills {
		if strings.EqualFold(strings.TrimSpace(sk), "memory") {
			return true
		}
	}
	return false
}

func memoryAction(a CustomAgent) string {
	if a.Memory != nil && strings.TrimSpace(a.Memory.Action) != "" {
		return strings.ToLower(strings.TrimSpace(a.Memory.Action))
	}
	return "shared"
}

func hydrateMemoryNodes(cw *CustomWorkflow, supervisor *agent.Supervisor, provider llm.Provider, state *SharedState) error {
	for _, a := range cw.Agents {
		if !isMemoryNode(a) {
			continue
		}
		action := memoryAction(a)
		switch action {
		case "shared", "workflow":
			state.WriteContract(a.Role+".memory_mode", "shared workflow memory only")
		case "read", "list":
			if err := memoryRead(a, supervisor, state); err != nil {
				return fmt.Errorf("%s read: %w", a.Role, err)
			}
		case "search":
			if err := memorySearch(a, supervisor, provider, state); err != nil {
				return fmt.Errorf("%s search: %w", a.Role, err)
			}
		case "write", "remember":
			if err := memoryWrite(a, supervisor, provider, state); err != nil {
				return fmt.Errorf("%s write: %w", a.Role, err)
			}
		case "read_write", "sync":
			if err := memoryRead(a, supervisor, state); err != nil {
				return fmt.Errorf("%s read: %w", a.Role, err)
			}
			if err := memoryWrite(a, supervisor, provider, state); err != nil {
				return fmt.Errorf("%s write: %w", a.Role, err)
			}
		default:
			state.WriteContract(a.Role+".memory_warning", fmt.Sprintf("unknown memory action %q; using shared workflow memory", action))
		}
	}
	return nil
}

func memoryLimit(a CustomAgent, fallback int) int {
	if a.Memory != nil && a.Memory.Limit > 0 {
		return a.Memory.Limit
	}
	return fallback
}

func memoryQuery(a CustomAgent, cwName string) string {
	if a.Memory != nil && strings.TrimSpace(a.Memory.Query) != "" {
		return strings.TrimSpace(a.Memory.Query)
	}
	for _, t := range a.Tasks {
		if strings.TrimSpace(t.Description) != "" {
			return strings.TrimSpace(t.Description)
		}
	}
	if strings.TrimSpace(a.Description) != "" {
		return strings.TrimSpace(a.Description)
	}
	return cwName
}

func memoryContent(a CustomAgent) string {
	if a.Memory != nil && strings.TrimSpace(a.Memory.Content) != "" {
		return strings.TrimSpace(a.Memory.Content)
	}
	parts := []string{}
	if strings.TrimSpace(a.Description) != "" {
		parts = append(parts, strings.TrimSpace(a.Description))
	}
	for _, t := range a.Tasks {
		if strings.TrimSpace(t.Description) != "" {
			parts = append(parts, strings.TrimSpace(t.Description))
		}
	}
	return strings.Join(parts, "\n")
}

func memoryRead(a CustomAgent, supervisor *agent.Supervisor, state *SharedState) error {
	store := supervisor.GetMemoryStore()
	if store == nil {
		state.WriteContract(a.Role+".workspace_memory", "Memory store tidak tersedia.")
		return nil
	}
	items, err := store.List(supervisor.GetWorkspaceID(), memoryLimit(a, 5))
	if err != nil {
		return err
	}
	state.WriteContract(a.Role+".workspace_memory", formatMemories(items))
	return nil
}

func memorySearch(a CustomAgent, supervisor *agent.Supervisor, provider llm.Provider, state *SharedState) error {
	store := supervisor.GetMemoryStore()
	if store == nil {
		state.WriteContract(a.Role+".workspace_memory", "Memory store tidak tersedia.")
		return nil
	}
	query := memoryQuery(a, "workspace")
	var results []memory.SearchResult
	p := provider
	if p == nil {
		p = supervisor.GetProvider()
	}
	if p != nil {
		if emb, err := p.GenerateEmbedding(query); err == nil && len(emb) > 0 {
			results, _ = store.Search(emb, supervisor.GetWorkspaceID(), memoryLimit(a, 5))
		}
	}
	if len(results) == 0 {
		fts := sanitizeMemoryFTS(query)
		if fts != "" {
			items, err := store.SearchFullText(fts, supervisor.GetWorkspaceID(), memory.MemoryFilters{Limit: memoryLimit(a, 5)})
			if err != nil {
				return err
			}
			for _, item := range items {
				results = append(results, memory.SearchResult{Memory: item})
			}
		}
	}
	items := make([]memory.Memory, 0, len(results))
	for _, r := range results {
		items = append(items, r.Memory)
	}
	state.WriteContract(a.Role+".workspace_memory_search", formatMemories(items))
	return nil
}

func memoryWrite(a CustomAgent, supervisor *agent.Supervisor, provider llm.Provider, state *SharedState) error {
	store := supervisor.GetMemoryStore()
	if store == nil {
		state.WriteContract(a.Role+".workspace_memory_write", "Memory store tidak tersedia.")
		return nil
	}
	content := memoryContent(a)
	if strings.TrimSpace(content) == "" {
		state.WriteContract(a.Role+".workspace_memory_write", "Tidak ada content untuk disimpan.")
		return nil
	}
	p := provider
	if p == nil {
		p = supervisor.GetProvider()
	}
	var emb []float32
	if p != nil {
		emb, _ = p.GenerateEmbedding(content)
	}
	mem, err := store.Save(content, "workflow,memory-node,"+a.Role, "custom_workflow", supervisor.GetWorkspaceID(), emb)
	if err != nil {
		return err
	}
	state.WriteContract(a.Role+".workspace_memory_write", fmt.Sprintf("Saved memory #%d", mem.ID))
	return nil
}

func formatMemories(items []memory.Memory) string {
	if len(items) == 0 {
		return "Tidak ada memori relevan."
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	var b strings.Builder
	for _, m := range items {
		b.WriteString(fmt.Sprintf("- #%d: %s\n", m.ID, strings.TrimSpace(m.Content)))
	}
	return strings.TrimSpace(b.String())
}

func sanitizeMemoryFTS(query string) string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	})
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 8 {
		fields = fields[:8]
	}
	return strings.Join(fields, " ")
}
