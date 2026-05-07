package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/agent/workflow"
	"github.com/gede-cahya/Smara-CLI/internal/config"
	"github.com/gede-cahya/Smara-CLI/internal/llm"
	"github.com/gede-cahya/Smara-CLI/internal/ui"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Kelola custom workflow dengan agent manual",
	Long:  `Buat, jalankan, edit, dan hapus custom workflow dengan definisi agent, skill, dan koneksi antar worker secara manual.`,
}

var workflowFile string
var workflowProjectDir string

var workflowCreateCmd = &cobra.Command{
	Use:   "create [nama]",
	Short: "Buat custom workflow baru",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		var cw *workflow.CustomWorkflow

		if workflowFile != "" {
			data, err := os.ReadFile(workflowFile)
			if err != nil {
				return fmt.Errorf("gagal membaca file: %w", err)
			}
			cw, err = workflow.CustomWorkflowFromJSON(data)
			if err != nil {
				return fmt.Errorf("gagal parse workflow JSON: %w", err)
			}
			cw.Name = name
		} else {
			// Interactive mode
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Deskripsi workflow '%s': ", name)
			desc, _ := reader.ReadString('\n')
			cw = &workflow.CustomWorkflow{
				Name:        name,
				Description: strings.TrimSpace(desc),
			}
			if workflowProjectDir != "" {
				cw.ProjectDir = workflowProjectDir
			}
			for {
				fmt.Print("Nama agent (kosongkan untuk selesai): ")
				role, _ := reader.ReadString('\n')
				role = strings.TrimSpace(role)
				if role == "" {
					break
				}
				fmt.Printf("Deskripsi agent '%s': ", role)
				adesc, _ := reader.ReadString('\n')
				fmt.Print("Skills (pisahkan koma): ")
				skillsStr, _ := reader.ReadString('\n')
				var skills []string
				for _, s := range strings.Split(skillsStr, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						skills = append(skills, s)
					}
				}
				fmt.Print("Depends on (pisahkan koma): ")
				depsStr, _ := reader.ReadString('\n')
				var deps []string
				for _, d := range strings.Split(depsStr, ",") {
					d = strings.TrimSpace(d)
					if d != "" {
						deps = append(deps, d)
					}
				}
				agent := workflow.CustomAgent{
					Role:        role,
					Description: strings.TrimSpace(adesc),
					Skills:      skills,
					DependsOn:   deps,
					Tasks: []workflow.Task{
						{ID: "main", Description: "Tugas utama untuk " + role},
					},
				}
				cw.Agents = append(cw.Agents, agent)
			}
		}

		if len(cw.Agents) == 0 {
			return fmt.Errorf("workflow harus memiliki minimal satu agent")
		}

		if err := workflow.SaveCustomWorkflow(cw); err != nil {
			return fmt.Errorf("gagal menyimpan workflow: %w", err)
		}
		ui.PrintSuccess("Custom workflow '%s' berhasil dibuat (%d agent)", cw.Name, len(cw.Agents))
		return nil
	},
}

var workflowListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Daftar custom workflow",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		names, err := workflow.ListCustomWorkflows()
		if err != nil {
			return fmt.Errorf("gagal list workflow: %w", err)
		}
		if len(names) == 0 {
			fmt.Println("Belum ada custom workflow.")
			return nil
		}
		fmt.Println("Custom Workflows:")
		for _, n := range names {
			cw, err := workflow.LoadCustomWorkflow(n)
			if err != nil {
				fmt.Printf("  - %s\n", n)
				continue
			}
			fmt.Printf("  - %s: %s (%d agent)\n", n, cw.Description, len(cw.Agents))
		}
		return nil
	},
}

var workflowRunCmd = &cobra.Command{
	Use:   "run [nama]",
	Short: "Jalankan custom workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cw, err := workflow.LoadCustomWorkflow(name)
		if err != nil {
			return fmt.Errorf("gagal load workflow '%s': %w", name, err)
		}
		if workflowProjectDir != "" {
			cw.ProjectDir = workflowProjectDir
		}

		cfg := config.Get()
		providerCfg := llm.ProviderConfig{
			Name:   cfg.Provider,
			Model:  cfg.Model,
			Host:   cfg.OllamaHost,
			APIKey: "",
		}
		switch cfg.Provider {
		case "openai":
			providerCfg.APIKey = cfg.OpenAIAPIKey
			providerCfg.Host = cfg.OpenAIBaseURL
		case "openrouter":
			providerCfg.APIKey = cfg.OpenRouterAPIKey
			if cfg.Model == "" || cfg.Model == "minimax-m2.5:cloud" {
				providerCfg.Model = cfg.OpenRouterModel
			}
		case "anthropic":
			providerCfg.APIKey = cfg.AnthropicAPIKey
			if cfg.Model == "" || cfg.Model == "minimax-m2.5:cloud" {
				providerCfg.Model = cfg.AnthropicModel
			}
		case "custom":
			providerCfg.APIKey = cfg.CustomAPIKey
			providerCfg.Host = cfg.CustomBaseURL
		}
		provider, err := llm.NewProvider(providerCfg)
		if err != nil {
			return fmt.Errorf("gagal inisialisasi provider: %w", err)
		}

		// Build lightweight supervisor
		supervisor, err := getSupervisorForSkill()
		if err != nil {
			return fmt.Errorf("gagal inisialisasi supervisor: %w", err)
		}
		defer supervisor.Close()

		ui.PrintInfo("Menjalankan custom workflow '%s'...", name)
		result, err := workflow.RunCustomWorkflow(supervisor, provider, cw)
		if err != nil {
			return fmt.Errorf("workflow gagal: %w", err)
		}
		ui.PrintSuccess("%s", result.FinalSummary)
		fmt.Printf("Project dir: %s\n", result.ProjectPath)
		return nil
	},
}

var workflowDeleteCmd = &cobra.Command{
	Use:   "delete [nama]",
	Short: "Hapus custom workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := workflow.DeleteCustomWorkflow(name); err != nil {
			return fmt.Errorf("gagal hapus workflow '%s': %w", name, err)
		}
		ui.PrintSuccess("Custom workflow '%s' dihapus.", name)
		return nil
	},
}

var workflowShowCmd = &cobra.Command{
	Use:   "show [nama]",
	Short: "Tampilkan detail custom workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cw, err := workflow.LoadCustomWorkflow(name)
		if err != nil {
			return fmt.Errorf("gagal load workflow '%s': %w", name, err)
		}
		data, _ := cw.ToJSON()
		fmt.Println(string(data))
		return nil
	},
}

var workflowImportCmd = &cobra.Command{
	Use:   "import [nama]",
	Short: "Import custom workflow dari file JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if workflowFile == "" {
			return fmt.Errorf("gunakan flag --file untuk file JSON yang mau di-import")
		}
		data, err := os.ReadFile(workflowFile)
		if err != nil {
			return fmt.Errorf("gagal membaca file: %w", err)
		}
		cw, err := workflow.CustomWorkflowFromJSON(data)
		if err != nil {
			return fmt.Errorf("gagal parse JSON: %w", err)
		}
		cw.Name = name
		if err := workflow.SaveCustomWorkflow(cw); err != nil {
			return fmt.Errorf("gagal menyimpan: %w", err)
		}
		ui.PrintSuccess("Workflow '%s' berhasil di-import (%d agent)", name, len(cw.Agents))
		return nil
	},
}

func init() {
	workflowCmd.AddCommand(workflowCreateCmd, workflowListCmd, workflowRunCmd, workflowDeleteCmd, workflowShowCmd, workflowImportCmd)
	workflowCreateCmd.Flags().StringVarP(&workflowFile, "file", "f", "", "File JSON definisi workflow")
	workflowCreateCmd.Flags().StringVar(&workflowProjectDir, "project-dir", "", "Direktori project (default: auto)")
	workflowRunCmd.Flags().StringVar(&workflowProjectDir, "project-dir", "", "Override direktori project")
	workflowImportCmd.Flags().StringVarP(&workflowFile, "file", "f", "", "File JSON yang mau di-import (wajib)")
	rootCmd.AddCommand(workflowCmd)
}
