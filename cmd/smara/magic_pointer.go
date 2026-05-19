package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gede-cahya/Smara-CLI/internal/magicpointer"
)

var magicPointerOpts struct {
	jsonOut        bool
	outputDir      string
	screenshotPath string
	keepScreenshot bool
	auditLog       string
	timeout        time.Duration
	instruction    string
	execute        bool
	yes            bool
	voice          bool
	voiceFile      string
	voiceDuration  time.Duration
	appMode        string
	privacyConfig  string
	privacyMode    string
	privacyBlock   string
	privacyAllow   string
	learning       bool
	autopilot      bool
	maxSteps       int
	stopCondition  string
}

var magicPointerCmd = &cobra.Command{
	Use:     "magic-pointer",
	Aliases: []string{"mpointer", "pointer-ai"},
	Short:   "Magic Pointer Phase 9: privacy dashboard, permission, learning behavior, dan eksekusi aman",
	Long: `Magic Pointer Phase 9 menjalankan AI screen context + voice/app-aware automation + privacy control + learning behavior.

Fitur:
- screenshot layar Linux via grim/gnome-screenshot/scrot/import jika tersedia,
- OCR via tesseract termasuk bounding box dari TSV,
- visual-lite: kandidat ikon, checkbox/radio, dropdown, input field berbasis layout,
- workflow multi-langkah dengan pemisah "lalu/kemudian/dan/;",
- Phase 6 voice command via --voice / --voice-file memakai whisper/whisper.cpp jika tersedia,
- Phase 7 app-aware mode: browser, code editor, email, spreadsheet, file manager, design,
- Phase 8 privacy dashboard/permission via --privacy-* dan blocklist app,
- Phase 9 learning user behavior via --learning,
- --ask untuk instruksi natural language, --execute untuk aksi aman memakai xdotool/ydotool,
- aksi sensitif/type perlu konfirmasi eksplisit via --yes,
- redaksi data sensitif dan audit log JSONL.`,
	RunE: func(cmd *cobra.Command, args []string) error { return runMagicPointerObserve(cmd.Context()) },
}

func runMagicPointerObserve(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, magicPointerOpts.timeout)
	defer cancel()

	if magicPointerOpts.privacyMode != "" || magicPointerOpts.privacyBlock != "" || magicPointerOpts.privacyAllow != "" {
		cfg, err := magicpointer.UpdatePrivacyConfig(magicPointerOpts.privacyConfig, magicPointerOpts.privacyMode, magicPointerOpts.privacyBlock, magicPointerOpts.privacyAllow, nil)
		if err != nil {
			return err
		}
		if magicPointerOpts.instruction == "" && !magicPointerOpts.voice && magicPointerOpts.voiceFile == "" {
			b, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Println(string(b))
			return nil
		}
	}

	audit := magicPointerOpts.auditLog
	if audit == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			audit = filepath.Join(home, ".smara", "magic-pointer", "audit.jsonl")
		}
	}
	baseOpts := magicpointer.Options{
		OutputDir: magicPointerOpts.outputDir, ScreenshotPath: magicPointerOpts.screenshotPath,
		KeepScreenshot: magicPointerOpts.keepScreenshot, RedactSensitive: true, AuditLogPath: audit,
		Instruction: magicPointerOpts.instruction, Execute: magicPointerOpts.execute, AssumeYes: magicPointerOpts.yes,
		Voice: magicPointerOpts.voice, VoiceFile: magicPointerOpts.voiceFile, VoiceDuration: magicPointerOpts.voiceDuration,
		AppMode: magicPointerOpts.appMode, PrivacyConfigPath: magicPointerOpts.privacyConfig, LearningEnabled: magicPointerOpts.learning,
	}
	if magicPointerOpts.autopilot {
		run, err := magicpointer.RunAutopilot(ctx, magicpointer.AutopilotOptions{Options: baseOpts, MaxSteps: magicPointerOpts.maxSteps, StopCondition: magicPointerOpts.stopCondition})
		if err != nil {
			return err
		}
		if magicPointerOpts.jsonOut {
			b, _ := json.MarshalIndent(run, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		fmt.Println("✨ Magic Pointer Phase 3 — Autopilot")
		fmt.Println("Instruction:", run.Instruction)
		fmt.Println("Completed  :", run.Completed)
		fmt.Println("StopReason :", run.StopReason)
		fmt.Println("Iterations :", len(run.Iterations), "/", run.MaxSteps)
		for i, it := range run.Iterations {
			if it.Plan != nil {
				fmt.Printf("%d. actions=%d executed=%d summary=%s\n", i+1, len(it.Plan.Actions), len(it.Plan.Executed), it.Plan.Summary)
			}
		}
		if audit != "" {
			fmt.Println("Audit log  :", audit)
		}
		return nil
	}
	sc, err := magicpointer.Observe(ctx, baseOpts)
	if err != nil {
		return err
	}
	if magicPointerOpts.jsonOut {
		b, _ := json.MarshalIndent(sc, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	title := "✨ Magic Pointer Phase 9 — Screen Context"
	if sc.Plan != nil {
		if magicPointerOpts.execute {
			title += " + Execute"
		} else {
			title += " + Safe Plan"
		}
	} else {
		title += " (read-only)"
	}
	fmt.Println(title)
	fmt.Println("Mode       :", sc.Mode)
	fmt.Println("Summary    :", sc.Summary)
	if sc.App.Profile != "" || sc.App.AppName != "" {
		fmt.Printf("App        : %s profile=%s title=%q\n", sc.App.AppName, sc.App.Profile, sc.App.WindowTitle)
	}
	if sc.Privacy != nil {
		fmt.Printf("Privacy   : mode=%s blocked=%v reason=%q\n", sc.Privacy.Mode, sc.Privacy.AppBlocked, sc.Privacy.Reason)
	}
	if sc.Learning != nil {
		fmt.Printf("Learning  : total_events=%d suggestions=%d\n", sc.Learning.TotalEvents, len(sc.Learning.SuggestedRoutines))
	}
	if sc.Voice != nil {
		fmt.Printf("Voice      : enabled transcript=%q tool=%s\n", sc.Voice.Transcript, sc.Voice.Tool)
	}
	if sc.ScreenshotPath != "" && magicPointerOpts.keepScreenshot {
		fmt.Println("Screenshot :", sc.ScreenshotPath)
	}
	if sc.ScreenshotHash != "" {
		fmt.Println("Hash       :", sc.ScreenshotHash)
	}
	if len(sc.Elements) > 0 {
		fmt.Println("\nElemen terdeteksi:")
		limit := len(sc.Elements)
		if limit > 12 {
			limit = 12
		}
		for i := 0; i < limit; i++ {
			e := sc.Elements[i]
			box := ""
			if e.Box != nil {
				box = fmt.Sprintf(" @(%d,%d %dx%d)", e.Box.X, e.Box.Y, e.Box.W, e.Box.H)
			}
			fmt.Printf("- [%s %.0f%%]%s %s\n", e.Type, e.Confidence*100, box, e.Text)
		}
	}
	if sc.Plan != nil {
		fmt.Println("\nRencana aksi:")
		fmt.Println("Instruksi :", sc.Plan.Instruction)
		fmt.Println("Summary   :", sc.Plan.Summary)
		for i, a := range sc.Plan.Actions {
			target := "-"
			if a.Target != nil {
				target = a.Target.Text
				if a.Target.Box != nil {
					target += fmt.Sprintf(" @(%d,%d %dx%d)", a.Target.Box.X, a.Target.Box.Y, a.Target.Box.W, a.Target.Box.H)
				}
			}
			fmt.Printf("%d. %s target=%q value=%q risk=%s confirm=%v\n   reason: %s\n", i+1, a.Type, target, a.Value, a.Risk, a.RequiresConfirmation, a.Reason)
		}
		if len(sc.Plan.Executed) > 0 {
			fmt.Println("\nEksekusi:")
			for i, e := range sc.Plan.Executed {
				status := "OK"
				if !e.Success {
					status = "FAIL: " + e.Error
				}
				fmt.Printf("%d. %s tool=%s target=%q @(%d,%d) status=%s\n", i+1, e.Type, e.Tool, e.Target, e.X, e.Y, status)
			}
		}
		for _, w := range sc.Plan.Warnings {
			fmt.Println("-", w)
		}
	}
	if len(sc.Warnings) > 0 {
		fmt.Println("\nWarning:")
		for _, w := range sc.Warnings {
			fmt.Println("-", w)
		}
	}
	if audit != "" {
		fmt.Println("\nAudit log  :", audit)
	}
	return nil
}

func init() {
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.jsonOut, "json", false, "Output JSON")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.outputDir, "output-dir", "", "Direktori screenshot/audio sementara/output")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.screenshotPath, "screenshot", "", "Gunakan screenshot yang sudah ada untuk observasi")
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.keepScreenshot, "keep-screenshot", false, "Simpan screenshot hasil capture")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.auditLog, "audit-log", "", "Path audit log JSONL")
	magicPointerCmd.Flags().DurationVar(&magicPointerOpts.timeout, "timeout", 20*time.Second, "Timeout observasi")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.instruction, "ask", "", "Instruksi natural language, misal: 'klik tombol login'")
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.execute, "execute", false, "Eksekusi aksi aman/workflow dengan xdotool/ydotool")
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.yes, "yes", false, "Konfirmasi aksi sensitif/type saat --execute")
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.voice, "voice", false, "Phase 6: rekam suara singkat lalu transkrip sebagai instruksi")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.voiceFile, "voice-file", "", "Phase 6: gunakan file audio existing untuk transkripsi")
	magicPointerCmd.Flags().DurationVar(&magicPointerOpts.voiceDuration, "voice-duration", 5*time.Second, "Durasi rekaman --voice")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.appMode, "app-mode", "auto", "Phase 7 app-aware mode: auto/browser/code_editor/email/spreadsheet/file_manager/design")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.privacyConfig, "privacy-config", "", "Phase 8: path config privacy Magic Pointer")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.privacyMode, "privacy-mode", "", "Phase 8: set privacy mode: normal/strict/allowlist")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.privacyBlock, "privacy-block-app", "", "Phase 8: tambahkan app/window ke blocklist privacy")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.privacyAllow, "privacy-allow-app", "", "Phase 8: tambahkan app/window ke allowlist privacy")
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.learning, "learning", false, "Phase 9: aktifkan learning behavior untuk instruksi ini")
	magicPointerCmd.Flags().BoolVar(&magicPointerOpts.autopilot, "autopilot", false, "Phase 3: jalankan observe-plan-execute-recover loop sampai selesai/stop")
	magicPointerCmd.Flags().IntVar(&magicPointerOpts.maxSteps, "max-steps", 10, "Phase 3: batas maksimum aksi/iterasi autopilot")
	magicPointerCmd.Flags().StringVar(&magicPointerOpts.stopCondition, "stop-condition", "", "Phase 3: teks kondisi berhenti jika muncul di observasi")
}
