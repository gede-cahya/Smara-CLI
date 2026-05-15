package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	smaraeval "github.com/gede-cahya/Smara-CLI/internal/eval"
)

var evalFile string
var evalJSON bool

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Jalankan evaluasi provider, skill, dan workflow",
}

var evalRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Jalankan eval suite terhadap provider aktif",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := providerFromConfig()
		if err != nil {
			return fmt.Errorf("gagal inisialisasi provider: %w", err)
		}
		suite := smaraeval.DefaultSuite()
		if evalFile != "" {
			suite, err = smaraeval.LoadSuite(evalFile)
			if err != nil {
				return err
			}
		}
		result := smaraeval.Run(provider, suite)
		if evalJSON {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("Eval suite: %s\n", result.Suite)
		fmt.Printf("Result: %d passed, %d failed, %d total\n", result.Passed, result.Failed, result.Total)
		for _, c := range result.Cases {
			status := "FAIL"
			if c.Passed {
				status = "PASS"
			}
			fmt.Printf("- %s [%s] %dms", c.Name, status, c.LatencyMs)
			if c.Error != "" {
				fmt.Printf(" error=%s", c.Error)
			}
			if len(c.Missing) > 0 {
				fmt.Printf(" missing=%v", c.Missing)
			}
			fmt.Println()
		}
		if result.Failed > 0 {
			return fmt.Errorf("eval gagal: %d case failed", result.Failed)
		}
		return nil
	},
}

func init() {
	evalRunCmd.Flags().StringVarP(&evalFile, "file", "f", "", "File JSON eval suite")
	evalRunCmd.Flags().BoolVar(&evalJSON, "json", false, "Output JSON")
	evalCmd.AddCommand(evalRunCmd)
	rootCmd.AddCommand(evalCmd)
}
