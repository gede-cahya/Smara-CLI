package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/skill"
)

// Flags for skill mock command.
var (
	skillMockPrefix  string
	skillMockCount   int
	skillMockClean   bool
	skillMockSeed    int64
	skillMockVerbose bool
)

// skillMockCmd generates ~100+ mock skills with realistic category, parent,
// dependency and lineage relationships so the web UI's hierarchy/tree view
// has meaningful content for demos and testing.
var skillMockCmd = &cobra.Command{
	Use:   "mock",
	Short: "Generate 100+ mock skill untuk demo & testing hierarchy view",
	Long: `Menulis sekitar 120 mock skill ke ~/.smara/skills/ dengan struktur:
  - 6 kategori besar (deploy, vps, monitoring, database, frontend, testing)
  - Setiap kategori: root skill → sub-skills → grandchildren
  - Beberapa skill punya refinement lineage (v1 → v2 → v3)
  - Dependency antar skill untuk constellation view

Semua mock skill diberi prefix "mock-" sehingga bisa dihapus dengan:
  smara skill mock --clean`,
	Run: func(cmd *cobra.Command, args []string) {
		if skillMockClean {
			cleanMockSkills(skillMockPrefix)
			return
		}
		generateMockSkills(skillMockPrefix, skillMockCount, skillMockSeed, skillMockVerbose)
	},
}

func init() {
	skillCmd.AddCommand(skillMockCmd)
	skillMockCmd.Flags().StringVar(&skillMockPrefix, "prefix", "mock-", "Prefix untuk semua mock skill (memudahkan cleanup)")
	skillMockCmd.Flags().IntVar(&skillMockCount, "count", 120, "Target minimum jumlah skill yang dibuat")
	skillMockCmd.Flags().Int64Var(&skillMockSeed, "seed", 0, "Random seed untuk variasi (0 = time-based)")
	skillMockCmd.Flags().BoolVar(&skillMockClean, "clean", false, "Hapus semua skill dengan prefix ini (kebalikan dari generate)")
	skillMockCmd.Flags().BoolVar(&skillMockVerbose, "verbose", false, "Print setiap skill yang dibuat")
}

// cleanMockSkills removes all skills whose name starts with the given prefix.
func cleanMockSkills(prefix string) {
	names, err := skill.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gagal baca skill list: %v\n", err)
		os.Exit(1)
	}
	deleted := 0
	for _, n := range names {
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		if err := skill.Delete(n, nil); err != nil {
			fmt.Fprintf(os.Stderr, "gagal hapus %s: %v\n", n, err)
			continue
		}
		deleted++
	}
	fmt.Printf("✓ Hapus %d mock skill dengan prefix '%s'\n", deleted, prefix)
}

// generateMockSkills creates the full mock skill forest.
func generateMockSkills(prefix string, target int, seed int64, verbose bool) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	created := 0

	// Each category definition: a name, a base step, and a set of sub-area
	// names. generateCategory expands this into a 3-level tree.
	categories := []mockCategory{
		{
			name:     "deploy",
			emoji:    "🚀",
			areas:    []string{"react", "nextjs", "static-site", "go-binary", "node-app", "docker"},
			tags:     []string{"deploy", "ci", "release"},
			baseStep: mockStep{Tool: "run_command", ArgsTemplate: map[string]string{"command": "echo deploying"}},
		},
		{
			name:     "vps",
			emoji:    "🖥️",
			areas:    []string{"systemctl", "pm2", "docker", "nginx", "certbot", "firewall"},
			tags:     []string{"vps", "ssh", "ops"},
			baseStep: mockStep{Tool: "ssh_exec", ArgsTemplate: map[string]string{"host": "vps-cahya", "command": "uptime"}},
		},
		{
			name:     "monitoring",
			emoji:    "📊",
			areas:    []string{"service-health", "disk-usage", "cpu-load", "memory", "logs", "alerts"},
			tags:     []string{"monitoring", "observability"},
			baseStep: mockStep{Tool: "ssh_exec", ArgsTemplate: map[string]string{"host": "vps-cahya", "command": "top -bn1"}},
		},
		{
			name:     "database",
			emoji:    "🗄️",
			areas:    []string{"postgres", "mysql", "sqlite", "redis", "backup", "restore"},
			tags:     []string{"database", "db", "backup"},
			baseStep: mockStep{Tool: "run_command", ArgsTemplate: map[string]string{"command": "pg_dump mydb"}},
		},
		{
			name:     "frontend",
			emoji:    "🎨",
			areas:    []string{"build", "lint", "test", "dev-server", "bundle-analysis", "i18n"},
			tags:     []string{"frontend", "web"},
			baseStep: mockStep{Tool: "run_command", ArgsTemplate: map[string]string{"command": "npm run build"}},
		},
		{
			name:     "testing",
			emoji:    "🧪",
			areas:    []string{"unit", "integration", "e2e", "smoke", "load", "contract"},
			tags:     []string{"testing", "qa"},
			baseStep: mockStep{Tool: "run_command", ArgsTemplate: map[string]string{"command": "go test ./..."}},
		},
	}

	// Create all skills. generateCategory returns the names of skills it
	// made so that dependencies can be wired up afterwards.
	var allNames []string
	nameToCategory := map[string]string{}

	for _, cat := range categories {
		names := generateCategory(prefix, cat, rng, verbose, &created)
		allNames = append(allNames, names...)
		for _, n := range names {
			nameToCategory[n] = cat.name
		}
	}

	// Wire up cross-cutting dependencies: every grandchild has a 30%
	// chance of depending on a peer or a sibling category's leaf.
	addDependencies(allNames, nameToCategory, rng, verbose)

	// Add top-up skills if we haven't reached the target count.
	for created < target {
		name := fmt.Sprintf("%sextra-%03d", prefix, created+1)
		sk := &skill.Skill{
			Name:        name,
			Description: "Utility skill tambahan untuk melengkapi tree.",
			Version:     1,
			Tags:        []string{"utility"},
			Steps: []skill.Step{
				{Tool: "run_command", Args: map[string]interface{}{"command": fmt.Sprintf("echo %s", name)}},
			},
			CategoryPath: []string{"utilities", "misc"},
		}
		if err := skill.Save(sk, nil); err == nil {
			created++
			if verbose {
				fmt.Printf("  + %s\n", name)
			}
		} else {
			break
		}
	}

	fmt.Printf("✓ Berhasil generate %d mock skill dengan prefix '%s'\n", created, prefix)
	fmt.Printf("  Seed: %d (gunakan --seed %d untuk reproduce)\n", seed, seed)
	fmt.Printf("  Hapus dengan: smara skill mock --clean --prefix %s\n", prefix)
	fmt.Printf("  Lihat di web: smara web  → tab Skill Tree → view Hierarchy\n")
}

type mockCategory struct {
	name     string
	emoji    string
	areas    []string // sub-areas become level 2 skills
	tags     []string
	baseStep mockStep
}

type mockStep struct {
	Tool         string
	ArgsTemplate map[string]string
}

// generateCategory creates a 3-level hierarchy for a single category:
//
//	<prefix><category>            (root, 1 skill)
//	├── <prefix><category>-<area> (sub-root, N skills)
//	│   ├── <prefix><category>-<area>-<verb> (leaf, 2-4 per sub-root)
//
// Also seeds lineage/refinement history on ~20% of skills.
func generateCategory(prefix string, cat mockCategory, rng *rand.Rand, verbose bool, createdCounter *int) []string {
	var names []string

	// Root skill
	rootName := prefix + cat.name
	rootSk := &skill.Skill{
		Name:        rootName,
		Description: fmt.Sprintf("%s Root skill untuk area %s.", cat.emoji, cat.name),
		Version:     1,
		Tags:        cat.tags,
		Steps: []skill.Step{
			{Tool: cat.baseStep.Tool, Args: templateToArgs(cat.baseStep.ArgsTemplate)},
		},
		CategoryPath: []string{cat.name},
	}
	// Root gets 2-4 refinement lineage entries to exercise the history UI.
	seedLineage(rootSk, 3, rng)

	if err := skill.Save(rootSk, nil); err == nil {
		names = append(names, rootName)
		*createdCounter++
		if verbose {
			fmt.Printf("  + %s (v%d, %d lineage)\n", rootName, rootSk.Version, len(rootSk.Lineage))
		}
	}

	// Sub-roots (level 2)
	verbs := []string{"run", "setup", "teardown", "status", "restart", "backup", "rollback", "init", "cleanup"}

	for _, area := range cat.areas {
		subName := fmt.Sprintf("%s%s-%s", prefix, cat.name, area)
		subSk := &skill.Skill{
			Name:         subName,
			Description:  fmt.Sprintf("Skill untuk %s dalam area %s.", area, cat.name),
			Version:      1,
			Tags:         append([]string{area}, cat.tags...),
			ParentID:     rootName,
			CategoryPath: []string{cat.name, area},
			Steps: []skill.Step{
				{Tool: cat.baseStep.Tool, Args: templateToArgs(cat.baseStep.ArgsTemplate)},
				{Tool: "run_command", Args: map[string]interface{}{"command": fmt.Sprintf("echo %s done", area)}},
			},
		}
		// 30% chance of refinement lineage on sub-roots
		if rng.Float64() < 0.3 {
			seedLineage(subSk, rng.Intn(3)+1, rng)
		}

		if err := skill.Save(subSk, nil); err == nil {
			names = append(names, subName)
			*createdCounter++
			if verbose {
				fmt.Printf("  + %s\n", subName)
			}
		}

		// Leaves (level 3)
		leafCount := 2 + rng.Intn(3) // 2-4 leaves per sub
		for i := 0; i < leafCount; i++ {
			verb := verbs[rng.Intn(len(verbs))]
			leafName := fmt.Sprintf("%s%s-%s-%s-%d", prefix, cat.name, area, verb, i+1)
			leafSk := &skill.Skill{
				Name:         leafName,
				Description:  fmt.Sprintf("%s %s untuk %s %s.", cat.emoji, verb, cat.name, area),
				Version:      1,
				Tags:         []string{verb, area},
				ParentID:     subName,
				CategoryPath: []string{cat.name, area, verb},
				Steps: []skill.Step{
					{Tool: "run_command", Args: map[string]interface{}{"command": fmt.Sprintf("%s-%s %s", area, verb, "__PARAM__target")}},
				},
				Params: []skill.ParamDef{
					{Name: "target", Type: "string", Description: "target resource name", Required: false, Default: "default"},
				},
			}
			// 15% chance of lineage on leaves
			if rng.Float64() < 0.15 {
				seedLineage(leafSk, 1, rng)
			}
			if err := skill.Save(leafSk, nil); err == nil {
				names = append(names, leafName)
				*createdCounter++
				if verbose {
					fmt.Printf("    + %s\n", leafName)
				}
			}
		}
	}

	return names
}

// seedLineage adds `count` fake prior-version entries to the skill's
// Lineage field so the history UI has content. The current Version is
// bumped accordingly.
func seedLineage(sk *skill.Skill, count int, rng *rand.Rand) {
	if count <= 0 {
		return
	}
	sources := []string{"auto", "manual", "feedback"}
	base := time.Now().Add(-time.Duration(count*7) * 24 * time.Hour)
	for i := 1; i <= count; i++ {
		sk.Lineage = append(sk.Lineage, skill.LineageEntry{
			Version:     i,
			Description: fmt.Sprintf("%s (versi lama v%d)", sk.Description, i),
			Tags:        sk.Tags,
			StepCount:   1 + rng.Intn(3),
			RefinedAt:   base.Add(time.Duration(i*24) * time.Hour),
			RefinedFrom: sources[rng.Intn(len(sources))],
		})
	}
	sk.Version = count + 1
}

// addDependencies wires up random cross-skill dependencies so the
// constellation & hierarchy views have some edges to render.
func addDependencies(allNames []string, nameToCategory map[string]string, rng *rand.Rand, verbose bool) {
	if len(allNames) < 5 {
		return
	}
	wired := 0
	for _, name := range allNames {
		// 25% chance of getting 1-2 dependencies (prefer same category)
		if rng.Float64() > 0.25 {
			continue
		}
		sk, err := skill.Load(name)
		if err != nil {
			continue
		}
		myCat := nameToCategory[name]
		var pool []string
		for _, other := range allNames {
			if other == name || nameToCategory[other] != myCat {
				continue
			}
			pool = append(pool, other)
		}
		if len(pool) == 0 {
			continue
		}
		depCount := 1
		if len(pool) > 3 && rng.Float64() < 0.3 {
			depCount = 2
		}
		deps := map[string]bool{}
		for len(deps) < depCount {
			deps[pool[rng.Intn(len(pool))]] = true
			if len(deps) >= len(pool) {
				break
			}
		}
		for d := range deps {
			sk.Dependencies = append(sk.Dependencies, d)
		}
		if err := skill.Save(sk, nil); err == nil {
			wired++
		}
	}
	if verbose {
		fmt.Printf("  ✓ Wired dependencies pada %d skill\n", wired)
	}
}

// templateToArgs converts a string→string map into the generic args map.
func templateToArgs(t map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(t))
	for k, v := range t {
		out[k] = v
	}
	return out
}
