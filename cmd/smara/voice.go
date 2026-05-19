package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/voice"
	"github.com/spf13/cobra"
)

var voiceOpts struct {
	text      string
	audio     string
	provider  string
	language  string
	output    string
	autopilot bool
	maxSteps  int
	jsonOut   bool
	timeout   time.Duration
}

var voiceCmd = &cobra.Command{
	Use:   "voice",
	Short: "Voice Assistant MVP: STT/TTS dan voice command planner",
}

var voiceSpeakCmd = &cobra.Command{
	Use:   "speak",
	Short: "Ucapkan teks memakai browser TTS metadata atau Piper jika tersedia",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), voiceOpts.timeout)
		defer cancel()
		res := voice.Synthesize(ctx, voice.SynthesisRequest{Text: voiceOpts.text, Output: voiceOpts.output, Settings: voice.Settings{Provider: voice.Provider(voiceOpts.provider), Language: voiceOpts.language}})
		if voiceOpts.jsonOut {
			b, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		fmt.Println("🔊 Voice TTS")
		fmt.Println("Provider:", res.Provider)
		if res.AudioPath != "" {
			fmt.Println("Audio   :", res.AudioPath)
		}
		if res.Error != "" {
			fmt.Println("Warning :", res.Error)
		}
		return nil
	},
}

var voicePlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Buat plan dari transkrip suara untuk chat atau Magic Pointer",
	RunE: func(cmd *cobra.Command, args []string) error {
		plan := voice.PlanCommand(voice.CommandRequest{Transcript: voiceOpts.text, Language: voiceOpts.language, Autopilot: voiceOpts.autopilot, MaxSteps: voiceOpts.maxSteps, Source: "cli"})
		if voiceOpts.jsonOut {
			b, _ := json.MarshalIndent(plan, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		fmt.Println("🎙️ Voice Command Plan")
		fmt.Println("Transcript:", plan.Transcript)
		fmt.Println("Intent    :", plan.Intent)
		if len(plan.MagicPointerArgs) > 0 {
			fmt.Println("CLI       : smara", joinArgs(plan.MagicPointerArgs))
		}
		for _, w := range plan.Warnings {
			fmt.Println("Warning  :", w)
		}
		return nil
	},
}

var voiceTranscribeCmd = &cobra.Command{
	Use:   "transcribe",
	Short: "Transkripsi audio dengan whisper/whisper.cpp atau mock",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), voiceOpts.timeout)
		defer cancel()
		text, tool, err := voice.Transcribe(ctx, voiceOpts.audio, voice.Provider(voiceOpts.provider), voiceOpts.language)
		out := map[string]string{"transcript": text, "tool": tool}
		if err != nil {
			out["error"] = err.Error()
		}
		if voiceOpts.jsonOut {
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		fmt.Println("🎧 Transcript:", text)
		fmt.Println("Tool       :", tool)
		if err != nil {
			fmt.Println("Error      :", err.Error())
		}
		return nil
	},
}

func joinArgs(args []string) string {
	out := ""
	for _, a := range args {
		if out == "" {
			out = a
		} else {
			out += " " + a
		}
	}
	return out
}

func init() {
	voiceCmd.AddCommand(voiceSpeakCmd, voicePlanCmd, voiceTranscribeCmd)
	voiceCmd.PersistentFlags().StringVar(&voiceOpts.provider, "provider", "browser", "Provider voice: browser/auto/whisper/piper/mock")
	voiceCmd.PersistentFlags().StringVar(&voiceOpts.language, "language", "id-ID", "Bahasa voice/STT/TTS")
	voiceCmd.PersistentFlags().BoolVar(&voiceOpts.jsonOut, "json", false, "Output JSON")
	voiceCmd.PersistentFlags().DurationVar(&voiceOpts.timeout, "timeout", 30*time.Second, "Timeout voice operation")
	voiceSpeakCmd.Flags().StringVar(&voiceOpts.text, "text", "", "Teks yang akan diucapkan")
	voiceSpeakCmd.Flags().StringVar(&voiceOpts.output, "output", "", "Output audio untuk Piper")
	voicePlanCmd.Flags().StringVar(&voiceOpts.text, "text", "", "Transkrip/perintah suara")
	voicePlanCmd.Flags().BoolVar(&voiceOpts.autopilot, "autopilot", false, "Rencanakan sebagai desktop autopilot jika cocok")
	voicePlanCmd.Flags().IntVar(&voiceOpts.maxSteps, "max-steps", 10, "Batas langkah Magic Pointer")
	voiceTranscribeCmd.Flags().StringVar(&voiceOpts.audio, "audio", "", "Path audio")
}
