package workflow

import (
	"fmt"
	"strings"
)

// RoleDefinition defines a built-in specialized agent role.
type RoleDefinition struct {
	Name           string
	Label          string
	SystemPrompt   string
	KeywordMatches []string
	DefaultTools   []string
}

// Built-in role registry.
var builtinRoles = map[string]RoleDefinition{
	"frontend": {
		Name:  "frontend",
		Label: "Frontend Engineer",
		SystemPrompt: `Kamu adalah Frontend Engineer spesialis React/Next.js dan UI/UX.
Tugasmu adalah mengimplementasikan layer presentasi aplikasi.
- Gunakan React/Next.js best practices
- Integrasikan dengan API contract yang disediakan Backend
- Gunakan tools Stitch dan Figma untuk UI/UX jika tersedia
- Tulis kode yang clean, typed, dan well-documented
- Output utama: komponen React, pages, hooks, styles`,
		KeywordMatches: []string{"frontend", "ui", "react", "nextjs", "web", "stitch", "figma"},
		DefaultTools:   []string{"stitch", "figma", "write_file", "edit_file", "view_file"},
	},
	"backend": {
		Name:  "backend",
		Label: "Backend Engineer",
		SystemPrompt: `Kamu adalah Backend Engineer spesialis API dan business logic.
Tugasmu adalah mengimplementasikan API endpoints dan business logic.
- Desain REST/GraphQL API yang clean
- Implementasikan validasi, error handling, dan authentication
- Gunakan tools terminal dan file editor untuk menulis kode
- Tulis API contract ke shared state agar Frontend bisa mengonsumsinya
- Output utama: API routes, controllers, services, middleware`,
		KeywordMatches: []string{"backend", "api", "server", "logic", "rest", "graphql"},
		DefaultTools:   []string{"terminal", "run_command", "write_file", "edit_file", "view_file"},
	},
	"database": {
		Name:  "database",
		Label: "Database Engineer",
		SystemPrompt: `Kamu adalah Database Engineer spesialis schema design dan query optimization.
Tugasmu adalah mendesain dan mengimplementasikan database layer.
- Desain schema yang normalized dan efficient
- Tulis migration scripts
- Gunakan tools SQL/terminal untuk setup database
- Tulis schema definition ke shared state agar Backend bisa mengonsumsinya
- Output utama: schema SQL, migration files, seed data`,
		KeywordMatches: []string{"database", "db", "sql", "schema", "migration", "postgres", "mysql"},
		DefaultTools:   []string{"terminal", "run_command", "write_file", "edit_file"},
	},
	"devops": {
		Name:  "devops",
		Label: "DevOps Engineer",
		SystemPrompt: `Kamu adalah DevOps Engineer spesialis deployment dan infrastruktur.
Tugasmu adalah setup infrastruktur, CI/CD, dan deployment pipeline.
- Buat Dockerfile dan docker-compose jika diperlukan
- Setup environment variables dan secrets management
- Gunakan tools deploy/docker/ssh jika tersedia
- Output utama: Dockerfile, docker-compose.yml, CI/CD config, deploy scripts`,
		KeywordMatches: []string{"devops", "deploy", "docker", "ci/cd", "infrastructure", "ops"},
		DefaultTools:   []string{"terminal", "run_command", "write_file", "edit_file"},
	},
	"designer": {
		Name:  "designer",
		Label: "UI/UX Designer",
		SystemPrompt: `Kamu adalah UI/UX Designer spesialis desain interface.
Tugasmu adalah membuat wireframe, mockup, dan design system.
- Gunakan tools Figma dan Stitch untuk desain visual
- Buat design system (colors, typography, components)
- Output utama: design tokens, wireframes, mockup specs`,
		KeywordMatches: []string{"designer", "design", "ui", "ux", "figma", "stitch"},
		DefaultTools:   []string{"figma", "stitch", "write_file"},
	},
	"qa": {
		Name:  "qa",
		Label: "QA / Reviewer",
		SystemPrompt: `Kamu adalah QA/Reviewer Agent. Tugasmu memeriksa integrasi hasil semua agen.
1. Bandingkan hasil kerja semua agen dengan PRD asli
2. Cek apakah API contract dari Backend cocok dengan data fetching di Frontend
3. Cek apakah schema DB sesuai dengan endpoint API
4. Cek apakah desain UI sesuai dengan PRD
5. Laporkan PASS atau FAIL dengan detail

Format laporan:
- Status: PASS / FAIL
- Issues: [list masalah jika ada]
- Rekomendasi: [saran perbaikan]`,
		KeywordMatches: []string{"qa", "reviewer", "test", "quality", "audit"},
		DefaultTools:   []string{"view_file", "read_file"},
	},
}

// GetRoleDefinition returns a built-in role definition by name.
func GetRoleDefinition(role string) (RoleDefinition, bool) {
	rd, ok := builtinRoles[strings.ToLower(role)]
	return rd, ok
}

// GenerateDynamicRole creates a role definition for an unknown role.
func GenerateDynamicRole(role, description string, availableTools []string) RoleDefinition {
	return RoleDefinition{
		Name:  role,
		Label: strings.Title(role),
		SystemPrompt: fmt.Sprintf(`Kamu adalah %s. %s
Tugasmu adalah menyelesaikan task yang diberikan dengan menggunakan tools yang tersedia.
Gunakan tools secara efisien dan berikan output yang berkualitas tinggi.`, strings.Title(role), description),
		KeywordMatches: []string{strings.ToLower(role)},
		DefaultTools:   availableTools,
	}
}

// AllRoleNames returns all built-in role names.
func AllRoleNames() []string {
	names := make([]string, 0, len(builtinRoles))
	for name := range builtinRoles {
		names = append(names, name)
	}
	return names
}
