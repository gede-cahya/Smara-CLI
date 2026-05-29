package imageflow

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
	"github.com/gede-cahya/Smara-CLI/internal/config"
)

type Workflow struct {
	Version  string   `json:"version"`
	Name     string   `json:"name"`
	Nodes    []Node   `json:"nodes"`
	Edges    []Edge   `json:"edges"`
	Metadata Metadata `json:"metadata"`
}

type Node struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Position Position               `json:"position"`
	Config   map[string]interface{} `json:"config"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Edge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	SourcePort string `json:"sourcePort,omitempty"`
	Target     string `json:"target"`
	TargetPort string `json:"targetPort,omitempty"`
}

type Metadata struct {
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type SharedWorkflow struct {
	Kind       string   `json:"kind"`
	Version    string   `json:"version"`
	ExportedAt string   `json:"exported_at"`
	Workflow   Workflow `json:"workflow"`
}

type Summary struct {
	Name      string `json:"name"`
	Nodes     int    `json:"nodes"`
	Edges     int    `json:"edges"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type RunResult struct {
	Status    string   `json:"status"`
	Path      string   `json:"path,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	ImageURL  string   `json:"image_url,omitempty"`
	ImageURLs []string `json:"image_urls,omitempty"`
	Model     string   `json:"model,omitempty"`
	Logs      []string `json:"logs"`
	Mode      string   `json:"mode,omitempty"`
	Cached    bool     `json:"cached,omitempty"`
}

type Asset struct {
	ID         string    `json:"id"`
	Workflow   string    `json:"workflow"`
	JobID      string    `json:"job_id"`
	Path       string    `json:"path"`
	ImageURL   string    `json:"image_url,omitempty"`
	Model      string    `json:"model,omitempty"`
	Mode       string    `json:"mode,omitempty"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Prompt     string    `json:"prompt,omitempty"`
	Seed       int       `json:"seed,omitempty"`
	SourceNode string    `json:"source_node,omitempty"`
	SizeBytes  int64     `json:"size_bytes,omitempty"`
	Archived   bool      `json:"archived,omitempty"`
	CreatedAt  string    `json:"created_at"`
	Metadata   *Workflow `json:"metadata,omitempty"`
}

type ModelInfo struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Quality          string `json:"quality"`
	OutputDir        string `json:"output_dir"`
	BaseURL          string `json:"base_url,omitempty"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	ImageCapable     bool   `json:"image_capable"`
	Status           string `json:"status"`
	Message          string `json:"message"`
}

type NodeRunStatus struct {
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

type Job struct {
	ID        string          `json:"id"`
	Workflow  string          `json:"workflow"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Nodes     []NodeRunStatus `json:"nodes"`
	Logs      []string        `json:"logs"`
	Result    *RunResult      `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	Attempts  int             `json:"attempts,omitempty"`
	Priority  int             `json:"priority,omitempty"`
}

type JobOptions struct {
	Priority int `json:"priority,omitempty"`
}

type jobRecord struct {
	job      Job
	workflow Workflow
	cancel   bool
	cancelFn context.CancelFunc
	retryOf  string
}

var jobs = struct {
	sync.Mutex
	items   map[string]*jobRecord
	running map[string]bool
}{
	items:   map[string]*jobRecord{},
	running: map[string]bool{},
}

const (
	maxConcurrentJobs = 2
	jobTimeout        = 10 * time.Minute
	maxImageDimension = 2048
)

func Save(wf *Workflow) error {
	if err := Validate(wf); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	if wf.Metadata.CreatedAt == "" {
		wf.Metadata.CreatedAt = now
	}
	wf.Metadata.UpdatedAt = now
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath(wf.Name), data, 0o644)
}

func Load(name string) (*Workflow, error) {
	data, err := os.ReadFile(filePath(name))
	if err != nil {
		return nil, fmt.Errorf("image flow %q tidak ditemukan: %w", name, err)
	}
	var wf Workflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func Delete(name string) error {
	return os.Remove(filePath(name))
}

func List() ([]Summary, error) {
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir())
	if err != nil {
		return nil, err
	}
	out := []Summary{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		wf, err := Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, Summary{Name: wf.Name, Nodes: len(wf.Nodes), Edges: len(wf.Edges), UpdatedAt: wf.Metadata.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

func StartJob(wf *Workflow) (*Job, error) {
	return StartJobWithOptions(wf, JobOptions{})
}

func StartJobWithOptions(wf *Workflow, opts JobOptions) (*Job, error) {
	if err := Validate(wf); err != nil {
		return nil, err
	}
	now := time.Now().Format(time.RFC3339)
	job := Job{
		ID:        fmt.Sprintf("imgflow-%d", time.Now().UnixNano()),
		Workflow:  wf.Name,
		Status:    "queued",
		CreatedAt: now,
		UpdatedAt: now,
		Nodes:     initialNodeStatuses(wf.Nodes),
		Logs:      []string{"Job queued."},
		Priority:  opts.Priority,
	}
	copyWF := *wf
	copyWF.Nodes = append([]Node{}, wf.Nodes...)
	copyWF.Edges = append([]Edge{}, wf.Edges...)

	jobs.Lock()
	jobs.items[job.ID] = &jobRecord{job: job, workflow: copyWF}
	canStart := len(jobs.running) < maxConcurrentJobs
	if canStart {
		jobs.running[job.ID] = true
	}
	snapshot := jobs.items[job.ID].job
	_ = persistJobsLocked()
	jobs.Unlock()
	appendAudit("job_queued", map[string]interface{}{"job_id": job.ID, "workflow": job.Workflow, "priority": job.Priority})

	if canStart {
		go runJob(job.ID)
	}
	return &snapshot, nil
}
func GetJob(id string) (*Job, bool) {
	jobs.Lock()
	defer jobs.Unlock()
	rec, ok := jobs.items[id]
	if !ok {
		return nil, false
	}
	snapshot := cloneJob(rec.job)
	return &snapshot, true
}

func CancelJob(id string) (*Job, bool) {
	jobs.Lock()
	defer jobs.Unlock()
	rec, ok := jobs.items[id]
	if !ok {
		return nil, false
	}
	if rec.job.Status == "queued" {
		rec.cancel = true
		rec.job.Status = "canceled"
		rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
		rec.job.Logs = append(rec.job.Logs, "Job canceled before execution.")
	} else if rec.job.Status == "running" {
		rec.cancel = true
		if rec.cancelFn != nil {
			rec.cancelFn()
		}
		rec.job.Logs = append(rec.job.Logs, "Cancel requested. The job will stop before the next node; provider calls that do not support cancellation may finish first.")
		rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	_ = persistJobsLocked()
	snapshot := cloneJob(rec.job)
	return &snapshot, true
}

func RetryJob(id string) (*Job, bool, error) {
	jobs.Lock()
	rec, ok := jobs.items[id]
	if !ok {
		jobs.Unlock()
		return nil, false, nil
	}
	if rec.job.Status != "failed" && rec.job.Status != "canceled" {
		snapshot := cloneJob(rec.job)
		jobs.Unlock()
		return &snapshot, true, fmt.Errorf("job %s belum bisa di-retry karena status=%s", id, rec.job.Status)
	}
	wf := rec.workflow
	attempts := rec.job.Attempts + 1
	priority := rec.job.Priority
	now := time.Now().Format(time.RFC3339)
	job := Job{ID: fmt.Sprintf("imgflow-%d", time.Now().UnixNano()), Workflow: wf.Name, Status: "queued", CreatedAt: now, UpdatedAt: now, Nodes: initialNodeStatuses(wf.Nodes), Logs: []string{fmt.Sprintf("Retry queued from %s.", id)}, Attempts: attempts, Priority: priority}
	jobs.items[job.ID] = &jobRecord{job: job, workflow: wf, retryOf: id}
	canStart := len(jobs.running) < maxConcurrentJobs
	if canStart {
		jobs.running[job.ID] = true
	}
	snapshot := cloneJob(job)
	_ = persistJobsLocked()
	jobs.Unlock()
	appendAudit("job_retry_queued", map[string]interface{}{"job_id": job.ID, "retry_of": id, "workflow": job.Workflow, "attempts": attempts, "priority": priority})
	if canStart {
		go runJob(job.ID)
	}
	return &snapshot, true, nil
}

func cloneJob(job Job) Job {
	snapshot := job
	snapshot.Nodes = append([]NodeRunStatus{}, job.Nodes...)
	snapshot.Logs = append([]string{}, job.Logs...)
	return snapshot
}
func Validate(wf *Workflow) error {
	if wf == nil {
		return fmt.Errorf("workflow kosong")
	}
	wf.Name = strings.TrimSpace(wf.Name)
	if wf.Name == "" {
		return fmt.Errorf("nama workflow wajib diisi")
	}
	if wf.Version == "" {
		wf.Version = "1.0"
	}
	if len(wf.Nodes) == 0 {
		return fmt.Errorf("workflow harus punya minimal satu node")
	}
	ids := map[string]Node{}
	for _, node := range wf.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return fmt.Errorf("node id kosong")
		}
		if _, exists := ids[node.ID]; exists {
			return fmt.Errorf("duplicate node id: %s", node.ID)
		}
		if !knownNodeType(node.Type) {
			return fmt.Errorf("node %s punya type tidak dikenal: %s", node.ID, node.Type)
		}
		ids[node.ID] = node
	}
	for _, edge := range wf.Edges {
		source, ok := ids[edge.Source]
		if !ok {
			return fmt.Errorf("edge %s source tidak ditemukan: %s", edge.ID, edge.Source)
		}
		target, ok := ids[edge.Target]
		if !ok {
			return fmt.Errorf("edge %s target tidak ditemukan: %s", edge.ID, edge.Target)
		}
		sourceType, ok := portType(source.Type, "output", edge.SourcePort)
		if !ok {
			return fmt.Errorf("edge %s source port tidak dikenal: %s.%s", edge.ID, source.Type, edge.SourcePort)
		}
		targetType, ok := portType(target.Type, "input", edge.TargetPort)
		if !ok {
			return fmt.Errorf("edge %s target port tidak dikenal: %s.%s", edge.ID, target.Type, edge.TargetPort)
		}
		if !compatiblePortTypes(sourceType, targetType) {
			return fmt.Errorf("edge %s tipe port tidak kompatibel: %s -> %s", edge.ID, sourceType, targetType)
		}
	}
	if err := validateResourceLimits(wf.Nodes); err != nil {
		return err
	}
	return nil
}

func validateResourceLimits(nodes []Node) error {
	for _, node := range nodes {
		switch node.Type {
		case "generate_image", "image_to_image", "inpaint", "outpaint", "upscale":
			width := intConfig(node.Config, "width", 1024)
			height := intConfig(node.Config, "height", 1024)
			if width > maxImageDimension || height > maxImageDimension {
				return fmt.Errorf("node %s resolusi terlalu besar: %dx%d, maksimum %dx%d", node.ID, width, height, maxImageDimension, maxImageDimension)
			}
			if width <= 0 || height <= 0 {
				return fmt.Errorf("node %s resolusi tidak valid: %dx%d", node.ID, width, height)
			}
		}
	}
	return nil
}

func knownNodeType(nodeType string) bool {
	_, ok := nodePorts[nodeType]
	return ok
}

func portType(nodeType, direction, port string) (string, bool) {
	directions, ok := nodePorts[nodeType]
	if !ok {
		return "", false
	}
	ports, ok := directions[direction]
	if !ok {
		return "", false
	}
	t, ok := ports[port]
	return t, ok
}

func compatiblePortTypes(sourceType, targetType string) bool {
	if sourceType == "" || targetType == "" {
		return false
	}
	return sourceType == targetType || sourceType == "any" || targetType == "any"
}

var nodePorts = map[string]map[string]map[string]string{
	"text_prompt":    {"input": {}, "output": {"prompt": "text"}},
	"batch_prompt":   {"input": {}, "output": {"prompt": "text"}},
	"model_loader":   {"input": {}, "output": {"model": "model"}},
	"image_input":    {"input": {}, "output": {"image": "image"}},
	"mask_input":     {"input": {}, "output": {"mask": "mask"}},
	"random_seed":    {"input": {}, "output": {"seed": "seed"}},
	"generate_image": {"input": {"prompt": "text", "model": "model", "seed": "seed"}, "output": {"image": "image"}},
	"image_to_image": {"input": {"image": "image", "prompt": "text", "model": "model", "seed": "seed"}, "output": {"image": "image"}},
	"inpaint":        {"input": {"image": "image", "mask": "mask", "prompt": "text", "model": "model", "seed": "seed"}, "output": {"image": "image"}},
	"outpaint":       {"input": {"image": "image", "prompt": "text", "model": "model", "seed": "seed"}, "output": {"image": "image"}},
	"upscale":        {"input": {"image": "image", "prompt": "text", "model": "model", "seed": "seed"}, "output": {"image": "image"}},
	"image_preview":  {"input": {"image": "image"}, "output": {"image": "image"}},
	"image_output":   {"input": {"image": "image"}, "output": {"asset": "json"}},
}

func initialNodeStatuses(nodes []Node) []NodeRunStatus {
	out := make([]NodeRunStatus, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, NodeRunStatus{NodeID: node.ID, Type: node.Type, Status: "idle"})
	}
	return out
}

func runJob(id string) {
	defer startNextQueuedJob(id)

	jobs.Lock()
	rec, ok := jobs.items[id]
	if !ok {
		jobs.Unlock()
		return
	}
	rec.job.Status = "running"
	rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
	rec.job.Logs = append(rec.job.Logs, "Job started.")
	wf := rec.workflow
	jobs.Unlock()

	for _, typ := range executionTypes(&wf) {
		if isCanceled(id) {
			updateJobCanceled(id)
			return
		}
		setNodesByType(id, typ, "running", "")
		if typ != executableNodeType(&wf) {
			setNodesByType(id, typ, "success", "ok")
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		setJobCancelFn(id, cancel)
		result, err := RunWithContext(ctx, &wf)
		setJobCancelFn(id, nil)
		if err != nil {
			if isCanceled(id) || err == context.Canceled {
				setNodesByType(id, typ, "skipped", "canceled")
				updateJobCanceled(id)
				return
			}
			setNodesByType(id, typ, "failed", err.Error())
			updateJobFailed(id, err)
			return
		}
		if isCanceled(id) {
			updateJobCanceled(id)
			return
		}
		setNodesByType(id, typ, "success", firstNonEmpty(result.Path, result.ImageURL, "generated"))
		setNodesByType(id, "image_preview", "success", firstNonEmpty(result.ImageURL, "ok"))
		setNodesByType(id, "image_output", "success", firstNonEmpty(result.Path, "ok"))
		updateJobSuccess(id, result)
		return
	}
}

func startNextQueuedJob(doneID string) {
	jobs.Lock()
	delete(jobs.running, doneID)
	nextIDs := []string{}
	for len(jobs.running)+len(nextIDs) < maxConcurrentJobs {
		nextID := nextQueuedJobIDLocked(nextIDs)
		if nextID == "" {
			break
		}
		jobs.running[nextID] = true
		nextIDs = append(nextIDs, nextID)
	}
	_ = persistJobsLocked()
	jobs.Unlock()
	for _, nextID := range nextIDs {
		go runJob(nextID)
	}
}

func nextQueuedJobIDLocked(skip []string) string {
	nextID := ""
	bestPriority := -1 << 30
	bestCreatedAt := ""
	for id, rec := range jobs.items {
		if rec.job.Status != "queued" || jobs.running[id] {
			continue
		}
		alreadyPicked := false
		for _, picked := range skip {
			if picked == id {
				alreadyPicked = true
				break
			}
		}
		if alreadyPicked {
			continue
		}
		if nextID == "" || rec.job.Priority > bestPriority || (rec.job.Priority == bestPriority && rec.job.CreatedAt < bestCreatedAt) {
			nextID = id
			bestPriority = rec.job.Priority
			bestCreatedAt = rec.job.CreatedAt
		}
	}
	return nextID
}

func setJobCancelFn(id string, cancel context.CancelFunc) {
	jobs.Lock()
	defer jobs.Unlock()
	if rec, ok := jobs.items[id]; ok {
		rec.cancelFn = cancel
	}
}

func isCanceled(id string) bool {
	jobs.Lock()
	defer jobs.Unlock()
	rec, ok := jobs.items[id]
	return ok && rec.cancel
}

func updateJobCanceled(id string) {
	jobs.Lock()
	defer jobs.Unlock()
	if rec, ok := jobs.items[id]; ok {
		rec.job.Status = "canceled"
		rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
		rec.job.Logs = append(rec.job.Logs, "Job canceled.")
		appendAudit("job_canceled", map[string]interface{}{"job_id": id, "workflow": rec.job.Workflow})
		for i := range rec.job.Nodes {
			if rec.job.Nodes[i].Status == "queued" || rec.job.Nodes[i].Status == "running" || rec.job.Nodes[i].Status == "idle" {
				rec.job.Nodes[i].Status = "skipped"
			}
		}
	}
}

func updateJobFailed(id string, err error) {
	jobs.Lock()
	defer jobs.Unlock()
	if rec, ok := jobs.items[id]; ok {
		rec.job.Status = "failed"
		rec.job.Error = err.Error()
		rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
		rec.job.Logs = append(rec.job.Logs, "Job failed: "+err.Error())
		appendAudit("job_failed", map[string]interface{}{"job_id": id, "workflow": rec.job.Workflow, "error": err.Error()})
	}
}
func updateJobSuccess(id string, result *RunResult) {
	var wf Workflow
	var workflowName string
	jobs.Lock()
	if rec, ok := jobs.items[id]; ok {
		rec.job.Status = "success"
		rec.job.Result = result
		rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
		rec.job.Logs = append(rec.job.Logs, result.Logs...)
		rec.job.Logs = append(rec.job.Logs, "Job completed.")
		wf = rec.workflow
		workflowName = rec.job.Workflow
		appendAudit("job_success", map[string]interface{}{"job_id": id, "workflow": rec.job.Workflow})
	}
	jobs.Unlock()
	paths := []string{}
	urls := []string{}
	if result != nil {
		paths = append(paths, result.Paths...)
		urls = append(urls, result.ImageURLs...)
		if len(paths) == 0 && result.Path != "" {
			paths = append(paths, result.Path)
			urls = append(urls, result.ImageURL)
		}
	}
	assetPrompt, assetSeed, assetProvider, assetSourceNode := assetMetadataFromWorkflow(&wf)
	for i, path := range paths {
		if path == "" {
			continue
		}
		imageURL := "/api/generated-image?path=" + url.QueryEscape(path)
		if i < len(urls) && urls[i] != "" {
			imageURL = urls[i]
		}
		seed := assetSeed
		if seed > 0 && i > 0 {
			seed += i
		}
		_ = SaveAsset(Asset{ID: fmt.Sprintf("asset-%d-%d", time.Now().UnixNano(), i), Workflow: workflowName, JobID: id, Path: path, ImageURL: imageURL, Model: result.Model, Mode: result.Mode, Provider: assetProvider, Prompt: assetPrompt, Seed: seed, SourceNode: assetSourceNode, CreatedAt: time.Now().Format(time.RFC3339), Metadata: &wf})
	}
}

func setNodesByType(id, typ, status, result string) {
	jobs.Lock()
	defer jobs.Unlock()
	rec, ok := jobs.items[id]
	if !ok {
		return
	}
	for i := range rec.job.Nodes {
		if rec.job.Nodes[i].Type == typ {
			rec.job.Nodes[i].Status = status
			rec.job.Nodes[i].Result = result
		}
	}
	rec.job.UpdatedAt = time.Now().Format(time.RFC3339)
	if result != "" {
		rec.job.Logs = append(rec.job.Logs, fmt.Sprintf("%s: %s (%s)", typ, status, result))
	} else {
		rec.job.Logs = append(rec.job.Logs, fmt.Sprintf("%s: %s", typ, status))
	}
}
func Run(wf *Workflow) (*RunResult, error) {
	return RunWithContext(context.Background(), wf)
}

func RunWithContext(ctx context.Context, wf *Workflow) (*RunResult, error) {
	if err := Validate(wf); err != nil {
		return nil, err
	}
	nodes := map[string]Node{}
	for _, node := range wf.Nodes {
		nodes[node.ID] = node
	}
	if node, ok := firstExecutableAdvancedNode(wf.Nodes); ok {
		return runAdvancedEdit(ctx, wf, nodes, node)
	}
	if edit, ok := firstNodeOfType(wf.Nodes, "image_to_image"); ok {
		return runImageEdit(ctx, wf, nodes, edit)
	}
	generate, ok := firstNodeOfType(wf.Nodes, "generate_image")
	if !ok {
		return nil, fmt.Errorf("node generate_image tidak ditemukan")
	}
	return runImageGenerate(ctx, wf, nodes, generate)
}

func firstNodeOfType(nodes []Node, typ string) (Node, bool) {
	for _, node := range nodes {
		if node.Type == typ {
			return node, true
		}
	}
	return Node{}, false
}

func upstreamNode(wf *Workflow, nodes map[string]Node, targetID, targetPort string) *Node {
	if wf == nil {
		return nil
	}
	for _, edge := range wf.Edges {
		if edge.Target != targetID || edge.TargetPort != targetPort {
			continue
		}
		node, ok := nodes[edge.Source]
		if !ok {
			return nil
		}
		return &node
	}
	return nil
}

func runAdvancedEdit(ctx context.Context, wf *Workflow, nodes map[string]Node, exec Node) (*RunResult, error) {
	promptNode := upstreamNode(wf, nodes, exec.ID, "prompt")
	imageNode := upstreamNode(wf, nodes, exec.ID, "image")
	maskNode := upstreamNode(wf, nodes, exec.ID, "mask")
	modelNode := upstreamNode(wf, nodes, exec.ID, "model")
	seedNode := upstreamNode(wf, nodes, exec.ID, "seed")
	prompts := promptVariantsFromNode(promptNode)
	if len(prompts) == 0 {
		prompt := stringConfig(exec.Config, "prompt", "")
		if exec.Type == "upscale" && prompt == "" {
			prompt = "upscale this image, preserve identity and composition, improve crisp detail"
		}
		if prompt != "" {
			prompts = []string{prompt}
		}
	}
	if len(prompts) == 0 {
		return nil, fmt.Errorf("%s prompt kosong", exec.Type)
	}
	imagePath := imagePathFromNode(imageNode)
	if imagePath == "" {
		return nil, fmt.Errorf("%s image input kosong", exec.Type)
	}
	maskPath := imagePathFromNode(maskNode)
	if exec.Type == "inpaint" && maskPath == "" {
		return nil, fmt.Errorf("inpaint mask input kosong")
	}
	model, provider, quality := modelConfig(modelNode)
	size := imageSize(exec)
	seed := seedFromNode(seedNode, intConfig(exec.Config, "seed", 0))
	logs := []string{
		fmt.Sprintf("Resolved %s image path from Image Input node.", exec.Type),
		fmt.Sprintf("Resolved %s prompt.", exec.Type),
		fmt.Sprintf("Using provider=%s model=%s size=%s quality=%s.", provider, model, size, quality),
	}
	if len(prompts) > 1 {
		logs = append(logs, fmt.Sprintf("Batch mode: running %d %s prompts.", len(prompts), exec.Type))
	}
	if maskPath != "" {
		logs = append(logs, "Using mask_path from Mask Input node.")
	}
	if seed > 0 {
		logs = append(logs, fmt.Sprintf("Using seed=%d.", seed))
	}
	paths := []string{}
	urls := []string{}
	for idx, prompt := range prompts {
		if exec.Type == "outpaint" {
			direction := stringConfig(exec.Config, "direction", "all")
			prompt = fmt.Sprintf("%s. Outpaint direction: %s.", prompt, direction)
		}
		if exec.Type == "upscale" {
			scale := intConfig(exec.Config, "scale", 2)
			prompt = fmt.Sprintf("%s. Upscale factor: %dx.", prompt, scale)
		}
		args := map[string]interface{}{
			"image_path": imagePath,
			"prompt":     prompt,
			"provider":   provider,
			"model":      model,
			"size":       size,
			"quality":    quality,
		}
		if maskPath != "" {
			args["mask_path"] = maskPath
		}
		if seed > 0 {
			args["seed"] = seed + idx
		}
		output, err := agent.ExecuteBuiltinToolWithContext(ctx, "edit_image", args, nil)
		if err != nil {
			return nil, err
		}
		logs = append(logs, fmt.Sprintf("Batch %d/%d complete.", idx+1, len(prompts)), output)
		path := extractGeneratedPath(output)
		if path != "" {
			paths = append(paths, path)
			urls = append(urls, "/api/generated-image?path="+url.QueryEscape(path))
		}
	}
	result := &RunResult{Status: "success", Paths: paths, ImageURLs: urls, Model: model, Logs: logs, Mode: exec.Type}
	if len(paths) > 0 {
		result.Path = paths[0]
		result.ImageURL = urls[0]
	}
	return result, nil
}

func runImageGenerate(ctx context.Context, wf *Workflow, nodes map[string]Node, generate Node) (*RunResult, error) {
	promptNode := upstreamNode(wf, nodes, generate.ID, "prompt")
	modelNode := upstreamNode(wf, nodes, generate.ID, "model")
	seedNode := upstreamNode(wf, nodes, generate.ID, "seed")
	prompts := promptVariantsFromNode(promptNode)
	if len(prompts) == 0 {
		return nil, fmt.Errorf("prompt kosong")
	}
	model, provider, quality := modelConfig(modelNode)
	size := imageSize(generate)
	seed := seedFromNode(seedNode, intConfig(generate.Config, "seed", 0))
	logs := []string{"Resolved prompt from Text Prompt node.", fmt.Sprintf("Using provider=%s model=%s size=%s quality=%s.", provider, model, size, quality)}
	if len(prompts) > 1 {
		logs = append(logs, fmt.Sprintf("Batch mode: running %d prompts.", len(prompts)))
	}
	if seed > 0 {
		logs = append(logs, fmt.Sprintf("Using seed=%d.", seed))
	}
	paths := []string{}
	urls := []string{}
	for idx, prompt := range prompts {
		args := map[string]interface{}{"prompt": prompt, "provider": provider, "model": model, "size": size, "quality": quality}
		if seed > 0 {
			args["seed"] = seed + idx
		}
		output, err := agent.ExecuteBuiltinToolWithContext(ctx, "generate_image", args, nil)
		if err != nil {
			return nil, err
		}
		logs = append(logs, fmt.Sprintf("Batch %d/%d complete.", idx+1, len(prompts)), output)
		path := extractGeneratedPath(output)
		if path != "" {
			paths = append(paths, path)
			urls = append(urls, "/api/generated-image?path="+url.QueryEscape(path))
		}
	}
	result := &RunResult{Status: "success", Paths: paths, ImageURLs: urls, Model: model, Logs: logs, Mode: "text-to-image"}
	if len(paths) > 0 {
		result.Path = paths[0]
		result.ImageURL = urls[0]
	}
	return result, nil
}

func runImageEdit(ctx context.Context, wf *Workflow, nodes map[string]Node, edit Node) (*RunResult, error) {
	promptNode := upstreamNode(wf, nodes, edit.ID, "prompt")
	imageNode := upstreamNode(wf, nodes, edit.ID, "image")
	modelNode := upstreamNode(wf, nodes, edit.ID, "model")
	seedNode := upstreamNode(wf, nodes, edit.ID, "seed")
	prompt := promptFromNode(promptNode)
	if prompt == "" {
		prompt = stringConfig(edit.Config, "prompt", "")
	}
	if prompt == "" {
		return nil, fmt.Errorf("edit prompt kosong")
	}
	imagePath := imagePathFromNode(imageNode)
	if imagePath == "" {
		return nil, fmt.Errorf("image input kosong")
	}
	model, provider, quality := modelConfig(modelNode)
	size := imageSize(edit)
	seed := seedFromNode(seedNode, intConfig(edit.Config, "seed", 0))
	args := map[string]interface{}{"image_path": imagePath, "prompt": prompt, "provider": provider, "model": model, "size": size, "quality": quality}
	if seed > 0 {
		args["seed"] = seed
	}
	logs := []string{"Resolved image path from Image Input node.", "Resolved edit prompt from Text Prompt node.", fmt.Sprintf("Using provider=%s model=%s size=%s quality=%s.", provider, model, size, quality)}
	if seed > 0 {
		logs = append(logs, fmt.Sprintf("Using seed=%d.", seed))
	}
	output, err := agent.ExecuteBuiltinToolWithContext(ctx, "edit_image", args, nil)
	if err != nil {
		return nil, err
	}
	path := extractGeneratedPath(output)
	result := &RunResult{Status: "success", Path: path, Model: model, Logs: append(logs, output), Mode: "image-to-image"}
	if path != "" {
		result.ImageURL = "/api/generated-image?path=" + url.QueryEscape(path)
	}
	return result, nil
}

func ListAssets() ([]Asset, error) {
	if err := os.MkdirAll(assetDir(), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(assetIndexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Asset{}, nil
		}
		return nil, err
	}
	var assets []Asset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, err
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].CreatedAt > assets[j].CreatedAt })
	return assets, nil
}

func SaveAsset(asset Asset) error {
	if strings.TrimSpace(asset.Path) == "" {
		return nil
	}
	if asset.ID == "" {
		asset.ID = fmt.Sprintf("asset-%d", time.Now().UnixNano())
	}
	if asset.CreatedAt == "" {
		asset.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if asset.ImageURL == "" {
		asset.ImageURL = "/api/generated-image?path=" + url.QueryEscape(asset.Path)
	}
	if asset.Width == 0 || asset.Height == 0 || asset.SizeBytes == 0 {
		width, height, sizeBytes := inspectImageAsset(asset.Path)
		if asset.Width == 0 {
			asset.Width = width
		}
		if asset.Height == 0 {
			asset.Height = height
		}
		if asset.SizeBytes == 0 {
			asset.SizeBytes = sizeBytes
		}
	}
	if err := os.MkdirAll(assetDir(), 0o755); err != nil {
		return err
	}
	assets, err := ListAssets()
	if err != nil {
		return err
	}
	assets = append([]Asset{asset}, assets...)
	seen := map[string]bool{}
	out := make([]Asset, 0, len(assets))
	for _, item := range assets {
		key := item.Path
		if key == "" {
			key = item.ID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= 100 {
			break
		}
	}
	return writeAssets(out)
}

func DeleteAsset(id string, deleteFile bool) (Asset, bool, error) {
	assets, err := ListAssets()
	if err != nil {
		return Asset{}, false, err
	}
	out := make([]Asset, 0, len(assets))
	var removed Asset
	found := false
	for _, asset := range assets {
		if asset.ID == id || asset.Path == id {
			removed = asset
			found = true
			continue
		}
		out = append(out, asset)
	}
	if !found {
		return Asset{}, false, nil
	}
	if err := writeAssets(out); err != nil {
		return Asset{}, false, err
	}
	if deleteFile && removed.Path != "" {
		_ = os.Remove(removed.Path)
	}
	return removed, true, nil
}

func ArchiveAsset(id string, archived bool) (Asset, bool, error) {
	assets, err := ListAssets()
	if err != nil {
		return Asset{}, false, err
	}
	found := false
	var updated Asset
	for i := range assets {
		if assets[i].ID == id || assets[i].Path == id {
			assets[i].Archived = archived
			updated = assets[i]
			found = true
			break
		}
	}
	if !found {
		return Asset{}, false, nil
	}
	if err := writeAssets(assets); err != nil {
		return Asset{}, false, err
	}
	return updated, true, nil
}

func CleanupAssets(maxAge time.Duration, deleteFiles bool) (int, error) {
	assets, err := ListAssets()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	kept := make([]Asset, 0, len(assets))
	removed := 0
	for _, asset := range assets {
		created, err := time.Parse(time.RFC3339, asset.CreatedAt)
		shouldRemove := asset.Archived && err == nil && created.Before(cutoff)
		if !shouldRemove {
			kept = append(kept, asset)
			continue
		}
		removed++
		if deleteFiles && isSafeImageAssetPath(asset.Path) {
			_ = os.Remove(asset.Path)
		}
	}
	if removed == 0 {
		return 0, nil
	}
	if err := writeAssets(kept); err != nil {
		return 0, err
	}
	return removed, nil
}

func isSafeImageAssetPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	allowed := []string{os.TempDir()}
	if home, err := os.UserHomeDir(); err == nil {
		allowed = append(allowed, filepath.Join(home, ".smara"))
	}
	for _, base := range allowed {
		baseAbs, err := filepath.Abs(base)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(baseAbs, abs)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func ModelStatus() ModelInfo {
	cfg := config.Get()
	provider := cfg.Provider
	model := cfg.ImageModel
	baseURL := ""
	apiKeyConfigured := false
	if provider == "" {
		provider = "custom"
	}
	if model == "" {
		model = "gpt-image-2"
	}
	switch strings.ToLower(provider) {
	case "openai":
		baseURL = cfg.OpenAIBaseURL
		apiKeyConfigured = strings.TrimSpace(cfg.OpenAIAPIKey) != ""
	case "custom":
		baseURL = cfg.CustomBaseURL
		apiKeyConfigured = strings.TrimSpace(cfg.CustomAPIKey) != ""
	default:
		apiKeyConfigured = true
	}
	status := "ready"
	message := "Image provider configured."
	if strings.ToLower(provider) == "openai" || strings.ToLower(provider) == "custom" {
		if !apiKeyConfigured {
			status = "missing_key"
			message = "API key belum dikonfigurasi untuk provider image."
		}
	}
	return ModelInfo{Provider: provider, Model: model, Quality: "high", OutputDir: cfg.ImageOutputDir, BaseURL: baseURL, APIKeyConfigured: apiKeyConfigured, ImageCapable: true, Status: status, Message: message}
}

func AvailableModels() []ModelInfo {
	current := ModelStatus()
	cfg := config.Get()
	models := []ModelInfo{current}
	add := func(provider, model, quality, baseURL string, keyConfigured bool) {
		if provider == "" || model == "" {
			return
		}
		for _, item := range models {
			if strings.EqualFold(item.Provider, provider) && item.Model == model {
				return
			}
		}
		status := "ready"
		message := "Available model preset."
		if (strings.EqualFold(provider, "openai") || strings.EqualFold(provider, "custom")) && !keyConfigured {
			status = "missing_key"
			message = "API key belum dikonfigurasi."
		}
		models = append(models, ModelInfo{Provider: provider, Model: model, Quality: quality, OutputDir: cfg.ImageOutputDir, BaseURL: baseURL, APIKeyConfigured: keyConfigured, ImageCapable: true, Status: status, Message: message})
	}
	openAIKey := strings.TrimSpace(cfg.OpenAIAPIKey) != ""
	customKey := strings.TrimSpace(cfg.CustomAPIKey) != ""
	add("openai", "gpt-image-2", "high", cfg.OpenAIBaseURL, openAIKey)
	add("openai", "gpt-image-1", "high", cfg.OpenAIBaseURL, openAIKey)
	add("custom", firstNonEmpty(cfg.ImageModel, "gpt-image-2"), "high", cfg.CustomBaseURL, customKey)
	return models
}

func writeAssets(assets []Asset) error {
	if err := os.MkdirAll(assetDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(assets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(assetIndexPath(), data, 0o644)
}

func inspectImageAsset(path string) (int, int, int64) {
	info, err := os.Stat(path)
	var sizeBytes int64
	if err == nil && !info.IsDir() {
		sizeBytes = info.Size()
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, sizeBytes
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, sizeBytes
	}
	return cfg.Width, cfg.Height, sizeBytes
}

func dir() string {
	base := filepath.Dir(config.Get().DBPath)
	if base == "." || base == "" {
		base = config.SmaraDir()
	}
	return filepath.Join(base, "image-flows")
}

func assetDir() string {
	base := filepath.Dir(config.Get().DBPath)
	if base == "." || base == "" {
		base = config.SmaraDir()
	}
	return filepath.Join(base, "image-flow-assets")
}

func auditLogPath() string {
	return filepath.Join(assetDir(), "audit.jsonl")
}

func appendAudit(event string, fields map[string]interface{}) {
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["event"] = event
	fields["time"] = time.Now().Format(time.RFC3339)
	data, err := json.Marshal(fields)
	if err != nil {
		return
	}
	_ = os.MkdirAll(assetDir(), 0o755)
	f, err := os.OpenFile(auditLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

func assetIndexPath() string {
	return filepath.Join(assetDir(), "index.json")
}

func filePath(name string) string {
	safe := strings.TrimSpace(name)
	safe = strings.ReplaceAll(safe, string(filepath.Separator), "-")
	safe = strings.ReplaceAll(safe, "/", "-")
	if safe == "" {
		safe = "workflow"
	}
	if filepath.Ext(safe) == "" {
		safe += ".json"
	}
	return filepath.Join(dir(), safe)
}

func promptFromNode(node *Node) string {
	if node == nil || node.Config == nil {
		return ""
	}
	if node.Type == "batch_prompt" {
		return batchPromptFromNode(node)
	}
	positive := stringConfig(node.Config, "positive", "")
	negative := stringConfig(node.Config, "negative", "")
	template := stringConfig(node.Config, "template", "{positive}")
	prompt := strings.ReplaceAll(template, "{positive}", positive)
	prompt = strings.ReplaceAll(prompt, "{negative}", negative)
	if !strings.Contains(template, "{negative}") && negative != "" {
		prompt += ". Avoid: " + negative
	}
	return strings.TrimSpace(prompt)
}

func batchPromptFromNode(node *Node) string {
	lines := strings.Split(stringConfig(node.Config, "prompts", ""), "\n")
	prompts := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			prompts = append(prompts, line)
		}
	}
	if len(prompts) == 0 {
		return ""
	}
	mode := strings.ToLower(stringConfig(node.Config, "mode", "first"))
	switch mode {
	case "random":
		return prompts[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(prompts))]
	case "all":
		return strings.Join(prompts, ". ")
	default:
		index := intConfig(node.Config, "index", 1)
		if index < 1 {
			index = 1
		}
		if index > len(prompts) {
			index = len(prompts)
		}
		return prompts[index-1]
	}
}

func promptVariantsFromNode(node *Node) []string {
	if node == nil || node.Config == nil {
		return nil
	}
	if node.Type != "batch_prompt" {
		prompt := promptFromNode(node)
		if prompt == "" {
			return nil
		}
		return []string{prompt}
	}
	lines := strings.Split(stringConfig(node.Config, "prompts", ""), "\n")
	prompts := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			prompts = append(prompts, line)
		}
	}
	if len(prompts) == 0 {
		return nil
	}
	mode := strings.ToLower(stringConfig(node.Config, "mode", "first"))
	if mode == "all" {
		return prompts
	}
	return []string{batchPromptFromNode(node)}
}

func imagePathFromNode(node *Node) string {
	if node == nil || node.Config == nil {
		return ""
	}
	if node.Type == "mask_input" {
		return stringConfig(node.Config, "mask_path", "")
	}
	return stringConfig(node.Config, "image_path", "")
}

func firstExecutableAdvancedNode(nodes []Node) (Node, bool) {
	for _, typ := range []string{"inpaint", "outpaint", "upscale"} {
		if node, ok := firstNodeOfType(nodes, typ); ok {
			return node, true
		}
	}
	return Node{}, false
}

func seedFromNode(node *Node, fallback int) int {
	if node == nil || node.Config == nil {
		return fallback
	}
	mode := strings.ToLower(stringConfig(node.Config, "mode", "fixed"))
	if mode == "random" {
		min := intConfig(node.Config, "min", 1)
		max := intConfig(node.Config, "max", 2147483647)
		if max < min {
			min, max = max, min
		}
		return rand.New(rand.NewSource(time.Now().UnixNano())).Intn(max-min+1) + min
	}
	return intConfig(node.Config, "seed", fallback)
}

func executableNodeType(wf *Workflow) string {
	if node, ok := firstExecutableAdvancedNode(wf.Nodes); ok {
		return node.Type
	}
	if _, ok := firstNodeOfType(wf.Nodes, "image_to_image"); ok {
		return "image_to_image"
	}
	return "generate_image"
}

func executionTypes(wf *Workflow) []string {
	prefix := []string{}
	if _, ok := firstNodeOfType(wf.Nodes, "batch_prompt"); ok {
		prefix = append(prefix, "batch_prompt")
	}
	if _, ok := firstNodeOfType(wf.Nodes, "random_seed"); ok {
		prefix = append(prefix, "random_seed")
	}
	if node, ok := firstExecutableAdvancedNode(wf.Nodes); ok {
		types := append(prefix, "image_input")
		if node.Type == "inpaint" {
			types = append(types, "mask_input")
		}
		return append(types, "text_prompt", "model_loader", node.Type, "image_preview", "image_output")
	}
	if _, ok := firstNodeOfType(wf.Nodes, "image_to_image"); ok {
		return append(prefix, "image_input", "text_prompt", "model_loader", "image_to_image", "image_preview", "image_output")
	}
	return append(prefix, "text_prompt", "model_loader", "generate_image", "image_preview", "image_output")
}

func assetMetadataFromWorkflow(wf *Workflow) (prompt string, seed int, provider string, sourceNode string) {
	if wf == nil {
		return "", 0, "", ""
	}
	for _, node := range wf.Nodes {
		if node.Type == "text_prompt" || node.Type == "batch_prompt" {
			n := node
			prompt = promptFromNode(&n)
			sourceNode = node.ID
			break
		}
	}
	for _, node := range wf.Nodes {
		if node.Type == "random_seed" {
			n := node
			seed = seedFromNode(&n, 0)
			break
		}
	}
	for _, node := range wf.Nodes {
		if node.Type == "model_loader" {
			n := node
			_, provider, _ = modelConfig(&n)
			break
		}
	}
	if provider == "" {
		_, provider, _ = modelConfig(nil)
	}
	return prompt, seed, provider, sourceNode
}

func modelConfig(node *Node) (model, provider, quality string) {
	cfg := config.Get()
	provider = cfg.Provider
	model = cfg.ImageModel
	quality = "high"
	if node != nil && node.Config != nil {
		provider = stringConfig(node.Config, "provider", provider)
		model = stringConfig(node.Config, "model", model)
		quality = stringConfig(node.Config, "quality", quality)
	}
	if provider == "" {
		provider = "custom"
	}
	if model == "" {
		model = "gpt-image-2"
	}
	return model, provider, quality
}

func imageSize(node Node) string {
	width := intConfig(node.Config, "width", 1024)
	height := intConfig(node.Config, "height", 1024)
	return fmt.Sprintf("%dx%d", width, height)
}

func stringConfig(values map[string]interface{}, key, fallback string) string {
	if value, ok := values[key]; ok {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case fmt.Stringer:
			return strings.TrimSpace(v.String())
		}
	}
	return fallback
}

func intConfig(values map[string]interface{}, key string, fallback int) int {
	if value, ok := values[key]; ok {
		switch v := value.(type) {
		case float64:
			if v > 0 {
				return int(v)
			}
		case int:
			if v > 0 {
				return v
			}
		case string:
			var parsed int
			if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return fallback
}

func extractGeneratedPath(output string) string {
	re := regexp.MustCompile(`(?m)^Path:\s*(.+)$`)
	match := re.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
