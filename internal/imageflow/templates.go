package imageflow

import (
	"fmt"
	"strings"
	"time"
)

type Template struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Workflow    Workflow `json:"workflow"`
}

type LintIssue struct {
	Level   string `json:"level"`
	NodeID  string `json:"node_id,omitempty"`
	EdgeID  string `json:"edge_id,omitempty"`
	Message string `json:"message"`
}

type WorkflowExplanation struct {
	Summary  string   `json:"summary"`
	Steps    []string `json:"steps"`
	Nodes    []string `json:"nodes"`
	Warnings []string `json:"warnings,omitempty"`
}

type OptimizationSuggestion struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

func BuiltinTemplates() []Template {
	return []Template{
		{ID: "text-to-image", Name: "Text to Image", Description: "Generate image dari prompt teks.", Tags: []string{"generate", "prompt"}, Workflow: workflowTextToImage("Text to Image", "A high quality image")},
		{ID: "image-edit", Name: "Image Edit", Description: "Edit image input dengan prompt.", Tags: []string{"edit", "image-to-image"}, Workflow: workflowImageEdit("Image Edit", "Edit gambar sesuai instruksi")},
		{ID: "inpaint", Name: "Inpaint", Description: "Edit area tertentu memakai mask.", Tags: []string{"edit", "mask", "inpaint"}, Workflow: workflowInpaint("Inpaint", "Perbaiki area yang dimask")},
		{ID: "outpaint", Name: "Outpaint", Description: "Perluas gambar ke arah tertentu.", Tags: []string{"edit", "outpaint"}, Workflow: workflowOutpaint("Outpaint", "Perluas gambar secara natural")},
		{ID: "upscale", Name: "Upscale", Description: "Tingkatkan resolusi/detail gambar.", Tags: []string{"upscale", "enhance"}, Workflow: workflowUpscale("Upscale", "Upscale dan perjelas detail")},
		{ID: "batch-prompt", Name: "Batch Prompt", Description: "Jalankan banyak prompt sekaligus.", Tags: []string{"batch", "generate"}, Workflow: workflowBatch("Batch Prompt")},
	}
}

func TemplateByID(id string) (Template, bool) {
	for _, tmpl := range BuiltinTemplates() {
		if tmpl.ID == id {
			return tmpl, true
		}
	}
	return Template{}, false
}

func NewWorkflowFromPrompt(name, instruction, prompt string) Workflow {
	if strings.TrimSpace(name) == "" {
		name = "Agent Generated Image Flow"
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = strings.TrimSpace(instruction)
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "A high quality image"
	}
	lower := strings.ToLower(instruction + " " + prompt)
	if strings.Contains(lower, "inpaint") || strings.Contains(lower, "mask") {
		return workflowInpaint(name, prompt)
	}
	if strings.Contains(lower, "outpaint") || strings.Contains(lower, "extend") || strings.Contains(lower, "perluas") {
		return workflowOutpaint(name, prompt)
	}
	if strings.Contains(lower, "upscale") || strings.Contains(lower, "enhance") || strings.Contains(lower, "perbesar") {
		return workflowUpscale(name, prompt)
	}
	if strings.Contains(lower, "edit") || strings.Contains(lower, "image to image") || strings.Contains(lower, "image-to-image") {
		return workflowImageEdit(name, prompt)
	}
	if strings.Contains(lower, "batch") || strings.Contains(lower, "banyak prompt") {
		return workflowBatch(name)
	}
	return workflowTextToImage(name, prompt)
}

func LintWorkflow(wf *Workflow) []LintIssue {
	issues := []LintIssue{}
	if err := Validate(wf); err != nil {
		issues = append(issues, LintIssue{Level: "error", Message: err.Error()})
	}
	if wf == nil {
		return issues
	}
	connected := map[string]map[string]bool{}
	for _, e := range wf.Edges {
		if connected[e.Target] == nil {
			connected[e.Target] = map[string]bool{}
		}
		connected[e.Target][e.TargetPort] = true
	}
	for _, n := range wf.Nodes {
		switch n.Type {
		case "generate_image":
			if !connected[n.ID]["prompt"] {
				issues = append(issues, LintIssue{Level: "warning", NodeID: n.ID, Message: "Generate Image belum menerima prompt."})
			}
			if !connected[n.ID]["model"] {
				issues = append(issues, LintIssue{Level: "info", NodeID: n.ID, Message: "Generate Image belum menerima model; default config akan dipakai."})
			}
		case "image_to_image", "inpaint", "outpaint", "upscale":
			if !connected[n.ID]["image"] {
				issues = append(issues, LintIssue{Level: "warning", NodeID: n.ID, Message: n.Type + " belum menerima image input."})
			}
			if n.Type == "inpaint" && !connected[n.ID]["mask"] {
				issues = append(issues, LintIssue{Level: "warning", NodeID: n.ID, Message: "Inpaint belum menerima mask input."})
			}
		}
	}
	return issues
}

func FixWorkflow(wf *Workflow) (*Workflow, []string) {
	if wf == nil {
		return nil, []string{"workflow kosong"}
	}
	fixed := *wf
	fixed.Nodes = append([]Node{}, wf.Nodes...)
	fixed.Edges = append([]Edge{}, wf.Edges...)
	actions := []string{}
	ids := map[string]bool{}
	for i := range fixed.Nodes {
		if fixed.Nodes[i].ID == "" {
			fixed.Nodes[i].ID = fmt.Sprintf("node-%d", i+1)
			actions = append(actions, "Mengisi node id kosong.")
		}
		ids[fixed.Nodes[i].ID] = true
		if fixed.Nodes[i].Config == nil {
			fixed.Nodes[i].Config = map[string]interface{}{}
		}
	}
	ensure := func(id, typ string, x, y float64, cfg map[string]interface{}) {
		if ids[id] {
			return
		}
		fixed.Nodes = append(fixed.Nodes, Node{ID: id, Type: typ, Position: Position{X: x, Y: y}, Config: cfg})
		ids[id] = true
		actions = append(actions, "Menambahkan node "+typ+".")
	}
	hasType := func(typ string) bool {
		for _, n := range fixed.Nodes {
			if n.Type == typ {
				return true
			}
		}
		return false
	}
	if !hasType("model_loader") {
		ensure("model-1", "model_loader", 0, 120, map[string]interface{}{})
	}
	if hasType("generate_image") && !hasType("text_prompt") && !hasType("batch_prompt") {
		ensure("prompt-1", "text_prompt", 0, 0, map[string]interface{}{"positive": "A high quality image"})
	}
	addEdge := func(id, s, sp, t, tp string) {
		for _, e := range fixed.Edges {
			if e.Source == s && e.SourcePort == sp && e.Target == t && e.TargetPort == tp {
				return
			}
		}
		fixed.Edges = append(fixed.Edges, Edge{ID: id, Source: s, SourcePort: sp, Target: t, TargetPort: tp})
		actions = append(actions, "Menghubungkan "+s+"."+sp+" ke "+t+"."+tp+".")
	}
	firstID := func(typ string) string {
		for _, n := range fixed.Nodes {
			if n.Type == typ {
				return n.ID
			}
		}
		return ""
	}
	promptID := firstID("text_prompt")
	if promptID == "" {
		promptID = firstID("batch_prompt")
	}
	modelID, imageID, maskID := firstID("model_loader"), firstID("image_input"), firstID("mask_input")
	for _, n := range fixed.Nodes {
		switch n.Type {
		case "generate_image":
			if promptID != "" {
				addEdge("fix-prompt-"+n.ID, promptID, "prompt", n.ID, "prompt")
			}
			if modelID != "" {
				addEdge("fix-model-"+n.ID, modelID, "model", n.ID, "model")
			}
		case "image_to_image", "outpaint", "upscale":
			if imageID != "" {
				addEdge("fix-image-"+n.ID, imageID, "image", n.ID, "image")
			}
			if promptID != "" && n.Type != "upscale" {
				addEdge("fix-prompt-"+n.ID, promptID, "prompt", n.ID, "prompt")
			}
			if modelID != "" {
				addEdge("fix-model-"+n.ID, modelID, "model", n.ID, "model")
			}
		case "inpaint":
			if imageID != "" {
				addEdge("fix-image-"+n.ID, imageID, "image", n.ID, "image")
			}
			if maskID != "" {
				addEdge("fix-mask-"+n.ID, maskID, "mask", n.ID, "mask")
			}
			if promptID != "" {
				addEdge("fix-prompt-"+n.ID, promptID, "prompt", n.ID, "prompt")
			}
			if modelID != "" {
				addEdge("fix-model-"+n.ID, modelID, "model", n.ID, "model")
			}
		}
	}
	return &fixed, actions
}

func ExplainWorkflow(wf *Workflow) WorkflowExplanation {
	if wf == nil {
		return WorkflowExplanation{Summary: "Workflow kosong."}
	}
	ex := WorkflowExplanation{Summary: fmt.Sprintf("Workflow %q berisi %d node dan %d edge.", wf.Name, len(wf.Nodes), len(wf.Edges))}
	for _, n := range wf.Nodes {
		ex.Nodes = append(ex.Nodes, fmt.Sprintf("%s: %s", n.ID, n.Type))
	}
	for _, typ := range executionTypes(wf) {
		ex.Steps = append(ex.Steps, "Execute "+typ)
	}
	for _, issue := range LintWorkflow(wf) {
		if issue.Level != "info" {
			ex.Warnings = append(ex.Warnings, issue.Message)
		}
	}
	return ex
}

func OptimizeWorkflow(wf *Workflow) []OptimizationSuggestion {
	s := []OptimizationSuggestion{{Level: "info", Message: "Gunakan seed tetap untuk hasil yang reproducible."}}
	if wf == nil {
		return s
	}
	for _, n := range wf.Nodes {
		if n.Type == "generate_image" || n.Type == "image_to_image" || n.Type == "inpaint" {
			if intConfig(n.Config, "width", 1024) > 1536 || intConfig(n.Config, "height", 1024) > 1536 {
				s = append(s, OptimizationSuggestion{Level: "warning", Message: "Resolusi besar bisa lambat; pertimbangkan generate kecil lalu upscale."})
			}
		}
	}
	return s
}

func ApplyParameters(wf *Workflow, params map[string]interface{}) Workflow {
	out := *wf
	out.Nodes = append([]Node{}, wf.Nodes...)
	for i := range out.Nodes {
		if out.Nodes[i].Config == nil {
			out.Nodes[i].Config = map[string]interface{}{}
		}
		switch out.Nodes[i].Type {
		case "text_prompt":
			if v, ok := params["prompt"]; ok {
				out.Nodes[i].Config["positive"] = v
			}
		case "batch_prompt":
			if v, ok := params["prompt"]; ok {
				out.Nodes[i].Config["prompts"] = v
			}
		case "random_seed", "generate_image", "image_to_image", "inpaint", "outpaint", "upscale":
			if v, ok := params["seed"]; ok {
				out.Nodes[i].Config["seed"] = v
			}
		}
	}
	return out
}

func baseWorkflow(name string, nodes []Node, edges []Edge) Workflow {
	now := time.Now().Format(time.RFC3339)
	return Workflow{Version: "1.0", Name: name, Nodes: nodes, Edges: edges, Metadata: Metadata{CreatedAt: now, UpdatedAt: now}}
}
func workflowTextToImage(name, prompt string) Workflow {
	return baseWorkflow(name, []Node{{ID: "prompt-1", Type: "text_prompt", Position: Position{X: 0, Y: 0}, Config: map[string]interface{}{"positive": prompt}}, {ID: "model-1", Type: "model_loader", Position: Position{X: 0, Y: 140}, Config: map[string]interface{}{}}, {ID: "generate-1", Type: "generate_image", Position: Position{X: 300, Y: 60}, Config: map[string]interface{}{"width": 1024, "height": 1024}}, {ID: "preview-1", Type: "image_preview", Position: Position{X: 600, Y: 20}, Config: map[string]interface{}{}}, {ID: "output-1", Type: "image_output", Position: Position{X: 600, Y: 170}, Config: map[string]interface{}{}}}, []Edge{{ID: "e1", Source: "prompt-1", SourcePort: "prompt", Target: "generate-1", TargetPort: "prompt"}, {ID: "e2", Source: "model-1", SourcePort: "model", Target: "generate-1", TargetPort: "model"}, {ID: "e3", Source: "generate-1", SourcePort: "image", Target: "preview-1", TargetPort: "image"}, {ID: "e4", Source: "generate-1", SourcePort: "image", Target: "output-1", TargetPort: "image"}})
}
func workflowImageEdit(name, prompt string) Workflow {
	return baseWorkflow(name, []Node{{ID: "image-1", Type: "image_input", Position: Position{X: 0, Y: 0}, Config: map[string]interface{}{}}, {ID: "prompt-1", Type: "text_prompt", Position: Position{X: 0, Y: 150}, Config: map[string]interface{}{"positive": prompt}}, {ID: "model-1", Type: "model_loader", Position: Position{X: 0, Y: 300}, Config: map[string]interface{}{}}, {ID: "edit-1", Type: "image_to_image", Position: Position{X: 320, Y: 120}, Config: map[string]interface{}{}}, {ID: "preview-1", Type: "image_preview", Position: Position{X: 640, Y: 80}, Config: map[string]interface{}{}}, {ID: "output-1", Type: "image_output", Position: Position{X: 640, Y: 230}, Config: map[string]interface{}{}}}, []Edge{{ID: "e1", Source: "image-1", SourcePort: "image", Target: "edit-1", TargetPort: "image"}, {ID: "e2", Source: "prompt-1", SourcePort: "prompt", Target: "edit-1", TargetPort: "prompt"}, {ID: "e3", Source: "model-1", SourcePort: "model", Target: "edit-1", TargetPort: "model"}, {ID: "e4", Source: "edit-1", SourcePort: "image", Target: "preview-1", TargetPort: "image"}, {ID: "e5", Source: "edit-1", SourcePort: "image", Target: "output-1", TargetPort: "image"}})
}
func workflowInpaint(name, prompt string) Workflow {
	wf := workflowImageEdit(name, prompt)
	wf.Nodes[3].ID = "inpaint-1"
	wf.Nodes[3].Type = "inpaint"
	wf.Nodes = append(wf.Nodes, Node{ID: "mask-1", Type: "mask_input", Position: Position{X: 0, Y: 440}, Config: map[string]interface{}{}})
	for i := 0; i < 3; i++ {
		wf.Edges[i].Target = "inpaint-1"
	}
	wf.Edges[3].Source = "inpaint-1"
	wf.Edges[4].Source = "inpaint-1"
	wf.Edges = append(wf.Edges, Edge{ID: "e6", Source: "mask-1", SourcePort: "mask", Target: "inpaint-1", TargetPort: "mask"})
	return wf
}
func workflowOutpaint(name, prompt string) Workflow {
	wf := workflowImageEdit(name, prompt)
	wf.Nodes[3].ID = "outpaint-1"
	wf.Nodes[3].Type = "outpaint"
	wf.Nodes[3].Config = map[string]interface{}{"direction": "all"}
	for i := 0; i < 3; i++ {
		wf.Edges[i].Target = "outpaint-1"
	}
	wf.Edges[3].Source = "outpaint-1"
	wf.Edges[4].Source = "outpaint-1"
	return wf
}
func workflowUpscale(name, prompt string) Workflow {
	wf := workflowImageEdit(name, prompt)
	wf.Nodes[3].ID = "upscale-1"
	wf.Nodes[3].Type = "upscale"
	wf.Nodes[3].Config = map[string]interface{}{"scale": 2, "prompt": prompt}
	for i := 0; i < 3; i++ {
		wf.Edges[i].Target = "upscale-1"
	}
	wf.Edges[3].Source = "upscale-1"
	wf.Edges[4].Source = "upscale-1"
	return wf
}
func workflowBatch(name string) Workflow {
	wf := workflowTextToImage(name, "cinematic landscape at sunrise\neditorial portrait on muted backdrop")
	wf.Nodes[0].Type = "batch_prompt"
	wf.Nodes[0].Config = map[string]interface{}{"prompts": "cinematic landscape at sunrise\neditorial portrait on muted backdrop", "mode": "all"}
	return wf
}
