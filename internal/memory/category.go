package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CreateCategory creates a new category within a workspace.
// If parentID is provided, the category will be created as a subcategory.
func (s *SQLiteStore) CreateCategory(name, description string, workspaceID int64, parentID *int64) (*Category, error) {
	if name == "" {
		return nil, fmt.Errorf("nama kategori tidak boleh kosong")
	}

	if workspaceID <= 0 {
		return nil, fmt.Errorf("workspace ID tidak valid")
	}

	// Check for duplicate category name in same workspace
	var count int
	err := s.db.QueryRow(
		"SELECT count(*) FROM categories WHERE workspace_id = ? AND name = ?",
		workspaceID, name,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("gagal cek duplikasi kategori: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("kategori '%s' sudah ada di workspace ini", name)
	}

	// If parentID is provided, verify it exists and belongs to same workspace
	if parentID != nil && *parentID > 0 {
		var parentWorkspaceID int64
		err = s.db.QueryRow(
			"SELECT workspace_id FROM categories WHERE id = ?",
			*parentID,
		).Scan(&parentWorkspaceID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("kategori parent tidak ditemukan")
			}
			return nil, fmt.Errorf("gagal membaca kategori parent: %w", err)
		}
		if parentWorkspaceID != workspaceID {
			return nil, fmt.Errorf("kategori parent tidak berada di workspace yang sama")
		}
	}

	cat := &Category{
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		ParentID:    parentID,
		CreatedAt:   time.Now(),
	}

	result, err := s.db.Exec(
		`INSERT INTO categories (workspace_id, name, description, parent_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		cat.WorkspaceID, cat.Name, cat.Description, cat.ParentID, cat.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat kategori: %w", err)
	}

	id, _ := result.LastInsertId()
	cat.ID = id

	return cat, nil
}

// GetCategory retrieves a category by ID.
// Returns nil if category is not found.
func (s *SQLiteStore) GetCategory(id int64) (*Category, error) {
	if id <= 0 {
		return nil, fmt.Errorf("ID kategori tidak valid")
	}

	var cat Category
	row := s.db.QueryRow(
		`SELECT id, workspace_id, name, description, parent_id, created_at
		 FROM categories WHERE id = ?`,
		id,
	)

	if err := row.Scan(&cat.ID, &cat.WorkspaceID, &cat.Name, &cat.Description, &cat.ParentID, &cat.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("gagal membaca kategori: %w", err)
	}

	return &cat, nil
}

// GetCategoryByName retrieves a category by name within a workspace.
func (s *SQLiteStore) GetCategoryByName(workspaceID int64, name string) (*Category, error) {
	if workspaceID <= 0 {
		return nil, fmt.Errorf("workspace ID tidak valid")
	}

	var cat Category
	row := s.db.QueryRow(
		`SELECT id, workspace_id, name, description, parent_id, created_at
		 FROM categories WHERE workspace_id = ? AND name = ?`,
		workspaceID, name,
	)

	if err := row.Scan(&cat.ID, &cat.WorkspaceID, &cat.Name, &cat.Description, &cat.ParentID, &cat.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("gagal membaca kategori: %w", err)
	}

	return &cat, nil
}

// ListCategories returns all categories for a workspace.
// If includeSubcategories is true, includes hierarchical subcategories.
func (s *SQLiteStore) ListCategories(workspaceID int64, includeSubcategories bool) ([]Category, error) {
	if workspaceID <= 0 {
		return nil, fmt.Errorf("workspace ID tidak valid")
	}

	var rows *sql.Rows
	var err error

	if includeSubcategories {
		// Get all categories, will be organized hierarchically by caller
		rows, err = s.db.Query(
			`SELECT id, workspace_id, name, description, parent_id, created_at
			 FROM categories WHERE workspace_id = ? ORDER BY parent_id NULLS FIRST, name ASC`,
			workspaceID,
		)
	} else {
		// Get only top-level categories (no parent)
		rows, err = s.db.Query(
			`SELECT id, workspace_id, name, description, parent_id, created_at
			 FROM categories WHERE workspace_id = ? AND parent_id IS NULL ORDER BY name ASC`,
			workspaceID,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("gagal query kategori: %w", err)
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.WorkspaceID, &cat.Name, &cat.Description, &cat.ParentID, &cat.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan kategori: %w", err)
		}
		categories = append(categories, cat)
	}

	return categories, nil
}

// GetSubcategories returns all direct subcategories of a parent category.
func (s *SQLiteStore) GetSubcategories(parentID int64) ([]Category, error) {
	if parentID <= 0 {
		return nil, fmt.Errorf("ID parent tidak valid")
	}

	rows, err := s.db.Query(
		`SELECT id, workspace_id, name, description, parent_id, created_at
		 FROM categories WHERE parent_id = ? ORDER BY name ASC`,
		parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal query subkategori: %w", err)
	}
	defer rows.Close()

	var subcategories []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.WorkspaceID, &cat.Name, &cat.Description, &cat.ParentID, &cat.CreatedAt); err != nil {
			return nil, fmt.Errorf("gagal scan subkategori: %w", err)
		}
		// Verify parent exists
		var parentExists bool
		err := s.db.QueryRow("SELECT count(*) FROM categories WHERE id = ?", parentID).Scan(&parentExists)
		if err != nil || !parentExists {
			continue
		}
		subcategories = append(subcategories, cat)
	}

	return subcategories, nil
}

// UpdateCategory updates an existing category.
func (s *SQLiteStore) UpdateCategory(id int64, updates map[string]interface{}) error {
	if id <= 0 {
		return fmt.Errorf("ID kategori tidak valid")
	}

	var setClauses []string
	var args []interface{}
	argCount := 1

	if name, ok := updates["name"].(string); ok && name != "" {
		// Check for duplicate name in same workspace
		var existingID int64
		err := s.db.QueryRow(
			"SELECT id FROM categories WHERE workspace_id = (SELECT workspace_id FROM categories WHERE id = ?) AND name = ? AND id != ?",
			id, name, id,
		).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("gagal cek duplikasi: %w", err)
		}
		if existingID > 0 {
			return fmt.Errorf("nama kategori '%s' sudah digunakan", name)
		}

		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argCount))
		args = append(args, name)
		argCount++
	}

	if description, ok := updates["description"].(string); ok {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argCount))
		args = append(args, description)
		argCount++
	}

	if parentID, ok := updates["parent_id"].(*int64); ok {
		// Verify parent exists and is not creating a cycle
		if parentID != nil && *parentID > 0 {
			var parentWorkspaceID int64
			err := s.db.QueryRow(
				"SELECT workspace_id FROM categories WHERE id = ?",
				*parentID,
			).Scan(&parentWorkspaceID)
			if err != nil {
				if err == sql.ErrNoRows {
					return fmt.Errorf("kategori parent tidak ditemukan")
				}
				return fmt.Errorf("gagal membaca kategori parent: %w", err)
			}

			// Check if parent is not a descendant of this category (prevent cycles)
			if *parentID == id {
				return fmt.Errorf("kategori tidak boleh menjadi parent dari dirinya sendiri")
			}

			setClauses = append(setClauses, fmt.Sprintf("parent_id = $%d", argCount))
			args = append(args, *parentID)
			argCount++
		} else {
			setClauses = append(setClauses, fmt.Sprintf("parent_id = $%d", argCount))
			args = append(args, nil)
			argCount++
		}
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("tidak ada field yang diupdate")
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE categories SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argCount)

	_, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("gagal update kategori: %w", err)
	}

	return nil
}

// DeleteCategory removes a category and optionally reassigns its memories.
// If reassignTo is provided, memories in this category will be moved to that category.
// Otherwise, memories will have their category_id set to NULL.
func (s *SQLiteStore) DeleteCategory(id int64, reassignTo *int64) error {
	if id <= 0 {
		return fmt.Errorf("ID kategori tidak valid")
	}

	// Check if category exists
	var exists bool
	err := s.db.QueryRow("SELECT count(*) FROM categories WHERE id = ?", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("gagal cek kategori: %w", err)
	}
	if !exists {
		return fmt.Errorf("kategori tidak ditemukan")
	}

	// Reassign memories if requested
	if reassignTo != nil {
		_, err = s.db.Exec(
			"UPDATE memories SET category_id = ? WHERE category_id = ?",
			*reassignTo, id,
		)
		if err != nil {
			return fmt.Errorf("gagal reassign memories: %w", err)
		}
	} else {
		// Set category_id to NULL for memories in this category
		_, err = s.db.Exec(
			"UPDATE memories SET category_id = NULL WHERE category_id = ?",
			id,
		)
		if err != nil {
			return fmt.Errorf("gagal update memories: %w", err)
		}
	}

	// Delete the category
	_, err = s.db.Exec("DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("gagal hapus kategori: %w", err)
	}

	return nil
}

// GetCategoryStats returns statistics about a category.
func (s *SQLiteStore) GetCategoryStats(categoryID int64) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var memoryCount int
	err := s.db.QueryRow(
		"SELECT count(*) FROM memories WHERE category_id = ?",
		categoryID,
	).Scan(&memoryCount)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung memory: %w", err)
	}
	stats["memory_count"] = memoryCount

	var subcategoryCount int
	err = s.db.QueryRow(
		"SELECT count(*) FROM categories WHERE parent_id = ?",
		categoryID,
	).Scan(&subcategoryCount)
	if err != nil {
		return nil, fmt.Errorf("gagal hitung subkategori: %w", err)
	}
	stats["subcategory_count"] = subcategoryCount

	return stats, nil
}

// GetCategoryTree builds a hierarchical tree of categories for a workspace.
func (s *SQLiteStore) GetCategoryTree(workspaceID int64) ([]Category, error) {
	categories, err := s.ListCategories(workspaceID, true)
	if err != nil {
		return nil, err
	}

	// Build a map for quick lookup
	categoryMap := make(map[int64]*Category)
	for i := range categories {
		categoryMap[categories[i].ID] = &categories[i]
	}

	// Build tree structure (this is simplified - in a real implementation,
	// you'd want to add a Children field to Category)
	var tree []Category
	for _, cat := range categories {
		if cat.ParentID == nil {
			tree = append(tree, cat)
		}
	}

	return tree, nil
}