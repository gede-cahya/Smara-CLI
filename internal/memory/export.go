package memory

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ExportFormat represents the format for exporting memories.
type ExportFormat string

const (
	ExportJSON ExportFormat = "json"
	ExportCSV  ExportFormat = "csv"
	ExportMD   ExportFormat = "markdown"
	ExportZIP  ExportFormat = "zip"
)

// ExportOptions contains options for exporting memories.
type ExportOptions struct {
	Format          ExportFormat
	IncludeEmbeddings bool
	IncludeMetadata   bool
	DateFrom        *time.Time
	DateTo          *time.Time
	Categories      []int64
	Tags            []string
}

// ExportMemories exports memories based on the provided options.
func (s *SQLiteStore) ExportMemories(workspaceID int64, options ExportOptions) ([]byte, string, error) {
	if workspaceID <= 0 {
		return nil, "", fmt.Errorf("workspace ID tidak valid")
	}

	// Build filters
	filters := MemoryFilters{
		Limit: 10000, // Export up to 10k memories at once
	}

	if options.DateFrom != nil {
		filters.DateFrom = options.DateFrom
	}
	if options.DateTo != nil {
		filters.DateTo = options.DateTo
	}
	if len(options.Categories) > 0 {
		filters.CategoryID = &options.Categories[0] // For simplicity, use first category
	}
	if len(options.Tags) > 0 {
		filters.Tags = options.Tags
	}

	// Get memories
	memories, _, err := s.ListMemoriesWithFilters(workspaceID, filters)
	if err != nil {
		return nil, "", fmt.Errorf("gagal ambil memory untuk export: %w", err)
	}

	if len(memories) == 0 {
		return nil, "", fmt.Errorf("tidak ada memory untuk diekspor")
	}

	var data []byte
	var filename string

	switch options.Format {
	case ExportJSON:
		data, filename, err = s.exportJSON(memories, options)
	case ExportCSV:
		data, filename, err = s.exportCSV(memories, options)
	case ExportMD:
		data, filename, err = s.exportMarkdown(memories, options)
	case ExportZIP:
		data, filename, err = s.exportZIP(memories, options)
	default:
		return nil, "", fmt.Errorf("format export tidak didukung: %s", options.Format)
	}

	if err != nil {
		return nil, "", err
	}

	return data, filename, nil
}

// exportJSON exports memories as JSON.
func (s *SQLiteStore) exportJSON(memories []Memory, options ExportOptions) ([]byte, string, error) {
	type exportMemory struct {
		ID          int64                  `json:"id"`
		WorkspaceID int64                  `json:"workspace_id"`
		CategoryID  *int64                 `json:"category_id,omitempty"`
		Content     string                 `json:"content"`
		Tags        []string               `json:"tags"`
		Source      string                 `json:"source"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
		CreatedAt   time.Time              `json:"created_at"`
		UpdatedAt   time.Time              `json:"updated_at"`
		ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
		Version     int                    `json:"version"`
		Embedding   []float32              `json:"embedding,omitempty"`
	}

	var exportMemories []exportMemory
	for _, m := range memories {
		em := exportMemory{
			ID:          m.ID,
			WorkspaceID: m.WorkspaceID,
			CategoryID:  m.CategoryID,
			Content:     m.Content,
			Tags:        m.Tags,
			Source:      m.Source,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
			ExpiresAt:   m.ExpiresAt,
			Version:     m.Version,
		}

		if options.IncludeMetadata {
			em.Metadata = m.Metadata
		}

		if options.IncludeEmbeddings {
			em.Embedding = m.Embedding
		}

		exportMemories = append(exportMemories, em)
	}

	data, err := json.MarshalIndent(exportMemories, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("gagal marshal JSON: %w", err)
	}

	filename := fmt.Sprintf("smara_memories_%d_%s.json", 
		memories[0].WorkspaceID, 
		time.Now().Format("20060102_150405"))

	return data, filename, nil
}

// exportCSV exports memories as CSV.
func (s *SQLiteStore) exportCSV(memories []Memory, options ExportOptions) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{"ID", "WorkspaceID", "Content", "Tags", "Source", "CreatedAt", "UpdatedAt"}
	if options.IncludeMetadata {
		header = append(header, "Metadata")
	}
	if options.IncludeEmbeddings {
		header = append(header, "Embedding")
	}
	if len(memories) > 0 && memories[0].CategoryID != nil {
		header = append(header, "CategoryID")
	}

	if err := writer.Write(header); err != nil {
		return nil, "", fmt.Errorf("gagal write header CSV: %w", err)
	}

	// Write data
	for _, m := range memories {
		record := []string{
			fmt.Sprintf("%d", m.ID),
			fmt.Sprintf("%d", m.WorkspaceID),
			m.Content,
			strings.Join(m.Tags, ";"),
			m.Source,
			m.CreatedAt.Format(time.RFC3339),
			m.UpdatedAt.Format(time.RFC3339),
		}

		if options.IncludeMetadata {
			metaJSON, _ := json.Marshal(m.Metadata)
			record = append(record, string(metaJSON))
		}

		if options.IncludeEmbeddings {
			embStr := ""
			if len(m.Embedding) > 0 {
				embBytes, _ := json.Marshal(m.Embedding)
				embStr = string(embBytes)
			}
			record = append(record, embStr)
		}

		if m.CategoryID != nil {
			record = append(record, fmt.Sprintf("%d", *m.CategoryID))
		}

		if err := writer.Write(record); err != nil {
			return nil, "", fmt.Errorf("gagal write record CSV: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", fmt.Errorf("gagal flush CSV: %w", err)
	}

	filename := fmt.Sprintf("smara_memories_%d_%s.csv", 
		memories[0].WorkspaceID, 
		time.Now().Format("20060102_150405"))

	return buf.Bytes(), filename, nil
}

// exportMarkdown exports memories as Markdown.
func (s *SQLiteStore) exportMarkdown(memories []Memory, options ExportOptions) ([]byte, string, error) {
	var sb strings.Builder
	
	sb.WriteString("# Smara Memory Export\n\n")
	sb.WriteString(fmt.Sprintf("**Workspace ID:** %d\n", memories[0].WorkspaceID))
	sb.WriteString(fmt.Sprintf("**Exported:** %s\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Total Memories:** %d\n\n", len(memories)))
	sb.WriteString("---\n\n")

	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("## Memory #%d\n\n", m.ID))
		
		sb.WriteString("| Field | Value |\n")
		sb.WriteString("|-------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Source | %s |\n", m.Source))
		sb.WriteString(fmt.Sprintf("| Created | %s |\n", m.CreatedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("| Updated | %s |\n", m.UpdatedAt.Format("2006-01-02 15:04")))
		
		if len(m.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("| Tags | %s |\n", strings.Join(m.Tags, ", ")))
		}
		
		if m.CategoryID != nil {
			sb.WriteString(fmt.Sprintf("| Category ID | %d |\n", *m.CategoryID))
		}
		
		if m.ExpiresAt != nil {
			sb.WriteString(fmt.Sprintf("| Expires | %s |\n", m.ExpiresAt.Format("2006-01-02")))
		}
		
		sb.WriteString(fmt.Sprintf("| Version | %d |\n", m.Version))
		sb.WriteString("|---|---|\n")
		sb.WriteString("\n")

		if options.IncludeMetadata && len(m.Metadata) > 0 {
			sb.WriteString("### Metadata\n\n")
			metaJSON, _ := json.MarshalIndent(m.Metadata, "", "  ")
			sb.WriteString("```json\n")
			sb.WriteString(string(metaJSON))
			sb.WriteString("\n```\n\n")
		}

		sb.WriteString("### Content\n\n")
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")

		if options.IncludeEmbeddings && len(m.Embedding) > 0 {
			sb.WriteString("### Embedding\n\n")
			sb.WriteString(fmt.Sprintf("*Dimension: %d*\n\n", len(m.Embedding)))
			sb.WriteString("```\n")
			for i, val := range m.Embedding {
				if i > 0 && i%10 == 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("%.4f ", val))
			}
			sb.WriteString("\n```\n\n")
		}

		sb.WriteString("---\n\n")
	}

	filename := fmt.Sprintf("smara_memories_%d_%s.md", 
		memories[0].WorkspaceID, 
		time.Now().Format("20060102_150405"))

	return []byte(sb.String()), filename, nil
}

// exportZIP exports memories as a ZIP archive containing multiple formats.
func (s *SQLiteStore) exportZIP(memories []Memory, options ExportOptions) ([]byte, string, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Export as JSON
	jsonData, _, err := s.exportJSON(memories, options)
	if err != nil {
		return nil, "", err
	}
	jsonFile, err := zipWriter.Create("memories.json")
	if err != nil {
		return nil, "", err
	}
	_, err = jsonFile.Write(jsonData)
	if err != nil {
		return nil, "", err
	}

	// Export as CSV
	csvData, _, err := s.exportCSV(memories, options)
	if err != nil {
		return nil, "", err
	}
	csvFile, err := zipWriter.Create("memories.csv")
	if err != nil {
		return nil, "", err
	}
	_, err = csvFile.Write(csvData)
	if err != nil {
		return nil, "", err
	}

	// Export as Markdown
	mdData, _, err := s.exportMarkdown(memories, options)
	if err != nil {
		return nil, "", err
	}
	mdFile, err := zipWriter.Create("memories.md")
	if err != nil {
		return nil, "", err
	}
	_, err = mdFile.Write(mdData)
	if err != nil {
		return nil, "", err
	}

	// Add README
	readme := "# Smara Memory Export\n\n" +
		"This archive contains exported memories from Smara CLI.\n\n" +
		"## Files\n\n" +
		"- **memories.json**: Complete export in JSON format (includes all fields)\n" +
		"- **memories.csv**: Tabular export in CSV format\n" +
		"- **memories.md**: Human-readable export in Markdown format\n\n" +
		"## Import\n\n" +
		"To import these memories back into Smara, use the JSON file:\n\n" +
		"```bash\n" +
		"smara memory import memories.json\n" +
		"```\n\n" +
		"## Generated\n\n" +
		time.Now().Format("2006-01-02 15:04:05") + "\n"
	
	readmeFile, err := zipWriter.Create("README.md")
	if err != nil {
		return nil, "", err
	}
	_, err = readmeFile.Write([]byte(readme))
	if err != nil {
		return nil, "", err
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("smara_memories_%d_%s.zip", 
		memories[0].WorkspaceID, 
		time.Now().Format("20060102_150405"))

	return buf.Bytes(), filename, nil
}

// ImportMemories imports memories from various formats.
func (s *SQLiteStore) ImportMemories(workspaceID int64, data []byte, format ExportFormat, options ImportOptions) (int, error) {
	if workspaceID <= 0 {
		return 0, fmt.Errorf("workspace ID tidak valid")
	}

	switch format {
	case ExportJSON:
		return s.importJSON(workspaceID, data, options)
	case ExportCSV:
		return s.importCSV(workspaceID, data, options)
	default:
		return 0, fmt.Errorf("format import tidak didukung: %s", format)
	}
}

// ImportOptions contains options for importing memories.
type ImportOptions struct {
	SkipDuplicates bool
	MergeMetadata  bool
	SourceOverride string
}

// importJSON imports memories from JSON data.
func (s *SQLiteStore) importJSON(workspaceID int64, data []byte, options ImportOptions) (int, error) {
	var memories []Memory
	if err := json.Unmarshal(data, &memories); err != nil {
		// Try unmarshaling as exportMemory format
		var exportMemories []struct {
			ID          int64                  `json:"id"`
			WorkspaceID int64                  `json:"workspace_id"`
			CategoryID  *int64                 `json:"category_id,omitempty"`
			Content     string                 `json:"content"`
			Tags        []string               `json:"tags"`
			Source      string                 `json:"source"`
			Metadata    map[string]interface{} `json:"metadata,omitempty"`
			CreatedAt   time.Time              `json:"created_at"`
			UpdatedAt   time.Time              `json:"updated_at"`
			ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
			Version     int                    `json:"version"`
			Embedding   []float32              `json:"embedding,omitempty"`
		}
		
		if err := json.Unmarshal(data, &exportMemories); err != nil {
			return 0, fmt.Errorf("gagal unmarshal JSON: %w", err)
		}
		
		// Convert to Memory
		for _, em := range exportMemories {
			memories = append(memories, Memory{
				ID:          em.ID,
				WorkspaceID: workspaceID, // Use target workspace
				CategoryID:  em.CategoryID,
				Content:     em.Content,
				Tags:        em.Tags,
				Source:      em.Source,
				Metadata:    em.Metadata,
				CreatedAt:   em.CreatedAt,
				UpdatedAt:   em.UpdatedAt,
				ExpiresAt:   em.ExpiresAt,
				Version:     em.Version,
				Embedding:   em.Embedding,
			})
		}
	}

	imported := 0
	for _, mem := range memories {
		// Override source if specified
		source := mem.Source
		if options.SourceOverride != "" {
			source = options.SourceOverride
		}

		// Check for duplicates if requested
		if options.SkipDuplicates {
			var exists int
			err := s.db.QueryRow(
				"SELECT count(*) FROM memories WHERE workspace_id = ? AND content = ?",
				workspaceID, mem.Content,
			).Scan(&exists)
			if err != nil || exists > 0 {
				continue
			}
		}

		// Merge metadata if requested
		metadata := mem.Metadata
		if options.MergeMetadata {
			var existingMetadata map[string]interface{}
			// Check if similar memory exists
			rows, err := s.db.Query(
				"SELECT metadata FROM memories WHERE workspace_id = ? AND content = ?",
				workspaceID, mem.Content,
			)
			if err == nil && rows.Next() {
				var metaJSON string
				if err := rows.Scan(&metaJSON); err == nil {
					json.Unmarshal([]byte(metaJSON), &existingMetadata)
				}
			}
			rows.Close()

			// Merge metadata
			if existingMetadata != nil {
				if metadata == nil {
					metadata = make(map[string]interface{})
				}
				for k, v := range existingMetadata {
					if _, exists := metadata[k]; !exists {
						metadata[k] = v
					}
				}
			}
		}

		// Import memory
		var categoryID *int64
		if mem.CategoryID != nil {
			categoryID = mem.CategoryID
		}

		var expiresAt *time.Time
		if mem.ExpiresAt != nil {
			expiresAt = mem.ExpiresAt
		}

		_, err := s.SaveWithOptions(
			mem.Content,
			strings.Join(mem.Tags, ","),
			source,
			workspaceID,
			mem.Embedding,
			categoryID,
			metadata,
			expiresAt,
		)
		if err != nil {
			continue
		}
		imported++
	}

	return imported, nil
}

// importCSV imports memories from CSV data.
func (s *SQLiteStore) importCSV(workspaceID int64, data []byte, options ImportOptions) (int, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	
	// Read header
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("gagal baca header CSV: %w", err)
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(col)] = i
	}

	imported := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // Skip malformed rows
		}

		if len(record) < 3 {
			continue // Skip incomplete rows
		}

		// Parse basic fields
		content := record[colIdx["content"]]
		tagsStr := record[colIdx["tags"]]
		source := record[colIdx["source"]]

		// Parse tags
		tags := strings.Split(tagsStr, ";")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}

		// Check for duplicates if requested
		if options.SkipDuplicates {
			var exists int
			err := s.db.QueryRow(
				"SELECT count(*) FROM memories WHERE workspace_id = ? AND content = ?",
				workspaceID, content,
			).Scan(&exists)
			if err != nil || exists > 0 {
				continue
			}
		}

		// Override source if specified
		if options.SourceOverride != "" {
			source = options.SourceOverride
		}

		// Import memory
		_, err = s.SaveWithOptions(
			content,
			strings.Join(tags, ","),
			source,
			workspaceID,
			nil, // No embedding from CSV
			nil, // No category
			nil, // No metadata
			nil, // No expiry
		)
		if err != nil {
			continue
		}
		imported++
	}

	return imported, nil
}

// GenerateExportFilename generates a filename for export.
func GenerateExportFilename(workspaceID int64, format ExportFormat) string {
	return fmt.Sprintf("smara_memories_%d_%s.%s",
		workspaceID,
		time.Now().Format("20060102_150405"),
		format)
}

// ValidateExportData validates export data before processing.
func ValidateExportData(memories []Memory) error {
	if len(memories) == 0 {
		return fmt.Errorf("tidak ada data untuk diekspor")
	}

	for _, m := range memories {
		if m.Content == "" {
			return fmt.Errorf("memory #%d tidak memiliki konten", m.ID)
		}
	}

	return nil
}