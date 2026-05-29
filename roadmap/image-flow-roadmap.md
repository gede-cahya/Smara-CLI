# Roadmap Fitur Image Flow

## Status Implementasi Saat Ini

Update 2026-05-23:

- Prototype **Image Flow** sudah dibuat di Smara Web sebagai halaman terpisah dari Custom Workflow.
- Frontend canvas memakai `@xyflow/react` dengan 5 node MVP: Text Prompt, Model Loader, Generate Image, Image Preview, dan Image Output.
- Workflow draft tersimpan di browser via `localStorage`.
- Workflow bisa import/export JSON schema v1 sederhana.
- Workflow bisa disimpan, dibuka, didaftar, dan dihapus lewat backend API `image-flow`.
- Graph validator awal sudah memeriksa required input dan kompatibilitas tipe port.
- Run awal sudah tersedia: backend membaca Text Prompt + Model Loader + Generate Image, lalu memanggil tool `generate_image`.
- Preview node bisa menampilkan output nyata dari `/api/generated-image` jika provider berhasil mengembalikan file gambar.
- Execution sudah async dengan single-job queue in-memory.
- UI melakukan polling status job dan memperbarui status node.
- Cancel request tersedia untuk job queued/running.
- Log panel run sudah tampil di inspector kanan.
- Image Input sekarang bisa upload file langsung dari Smara Web.
- Preview lokal bisa membaca gambar upload Smara dari `~/.smara/clip-images`.
- Panel Preview sudah menampilkan input dan output berdampingan untuk before/after sederhana.

Belum selesai:

- Cancel belum bisa memutus provider HTTP call yang sudah berjalan; status dibatalkan setelah call selesai/terdeteksi.
- Retry failed node belum ada.
- Status realtime masih polling, belum WebSocket/SSE.
- Preview output belum punya history/gallery khusus Image Flow.
- Belum ada model manager dan advanced image editing nodes.

Phase 3 awal:

- Image Input Node sudah tersedia untuk path gambar lokal.
- Image Edit / image-to-image node sudah tersedia.
- Backend memilih alur `edit_image` jika workflow memiliki node `image_to_image`.
- Template cepat **Edit** sudah tersedia di panel node library.
- Upload file langsung ke Image Input sudah tersedia.
- Before/after compare sederhana sudah tersedia di panel Preview.
- Random Seed node sudah tersedia dan bisa dikoneksikan ke Generate/Edit sebagai input number.
- Batch Prompt node sudah tersedia dan bisa mengirim prompt multiline ke Generate/Edit/Inpaint.
- Mask Input node sudah tersedia dengan upload mask langsung dari inspector.
- Mask editor kanvas sudah tersedia di inspector Mask Input dan bisa menyimpan PNG mask ke `mask_path`.
- Inpaint, Outpaint, dan Upscale node sudah tersedia sebagai alur image-edit provider.
- Tool `edit_image` sekarang menerima `mask_path` dan mengirim multipart `mask` saat provider mendukungnya.

Belum selesai Phase 3:

- Phase 3 MVP sudah selesai untuk target roadmap saat ini.
- Catatan non-blocking: inpaint/outpaint/upscale masih memakai capability image-edit provider. Provider lokal khusus bisa dikerjakan sebagai perluasan Phase 4/6 setelah model/provider manager dipilih.

## 1. Ringkasan

**Image Flow** adalah fitur node-based workflow untuk membuat, mengedit, mengatur, dan mengeksekusi proses generasi gambar secara visual. Inspirasi utama berasal dari pengalaman kerja seperti ComfyUI: pengguna dapat menyusun node di canvas, menghubungkan input-output antar node, menjalankan workflow, melihat preview, menyimpan preset, dan mengelola model/asset.

Target utama fitur ini bukan sekadar meniru ComfyUI, tetapi membuat sistem workflow visual yang lebih terintegrasi dengan aplikasi Smara: mudah dipakai, bisa diperluas, bisa diotomasi oleh agent, dan mendukung image generation/editing modern.

---

## 2. Visi Produk

Membuat fitur **Image Flow** yang memungkinkan pengguna membangun pipeline visual untuk image generation/editing tanpa harus menulis kode.

Pengguna ideal:

- Creator yang ingin membuat workflow gambar reusable.
- Developer/AI engineer yang ingin menguji pipeline image model.
- Tim produk yang ingin membuat template otomatis untuk prompt, style, image editing, upscaling, dan batch generation.
- Agent Smara yang dapat membantu membuat, memperbaiki, dan menjalankan workflow.

Outcome jangka panjang:

- Workflow bisa dibuat manual di canvas.
- Workflow bisa dibuat otomatis dari prompt user.
- Workflow bisa disimpan sebagai template.
- Workflow bisa dieksekusi lokal atau remote.
- Workflow bisa diintegrasikan dengan image model, asset manager, dan automation skill.

---

## 3. Prinsip Desain

1. **Node-first**  
   Semua proses image pipeline direpresentasikan sebagai node dan edge.

2. **Visual, tapi tetap teknis**  
   Pengguna awam bisa drag-and-drop, pengguna teknis tetap bisa melihat JSON/schema detail.

3. **Composable**  
   Node kecil dapat digabung menjadi pipeline kompleks.

4. **Inspectable**  
   Setiap node punya input, output, log, status, error, dan preview.

5. **Agent-friendly**  
   Agent bisa membaca workflow, menyarankan node, memperbaiki koneksi, dan membuat workflow dari prompt.

6. **Non-destructive**  
   Workflow, image output, dan konfigurasi model harus bisa dilacak dan dipulihkan.

7. **Extensible**  
   Node baru dapat ditambahkan lewat registry/plugin.

---

## 4. Scope Fitur Utama

### 4.1 Canvas Workflow

Fitur canvas utama:

- Infinite canvas.
- Pan dan zoom.
- Drag node.
- Connect antar port node.
- Multi-select.
- Copy/paste node.
- Delete node/edge.
- Auto-layout sederhana.
- Minimap.
- Group node.
- Comment/sticky note.
- Fit-to-view.

### 4.2 Node System

Node minimal untuk MVP:

- **Text Prompt Node**
  - Positive prompt.
  - Negative prompt.
  - Prompt template.

- **Model Loader Node**
  - Checkpoint/model selection.
  - Provider selection.
  - Model metadata.

- **Image Input Node**
  - Upload image.
  - Use previous output.
  - Clipboard input.

- **Sampler / Generate Node**
  - Seed.
  - Steps.
  - CFG scale.
  - Width/height.
  - Scheduler/sampler.

- **Image Preview Node**
  - Display output image.
  - Compare before/after.
  - Save/export action.

- **Image Output Node**
  - Save to gallery.
  - Export file.
  - Send to other app/module.

Node lanjutan:

- LoRA node.
- ControlNet node.
- IP Adapter node.
- Inpaint node.
- Outpaint node.
- Upscale node.
- Face restore node.
- Background remove node.
- Image blend/composite node.
- Mask editor node.
- Batch prompt node.
- Random seed node.
- Metadata extractor node.
- Conditional/router node.
- Script/custom code node.
- API call node.

---

## 5. Workflow Execution

### 5.1 Execution Model

Workflow dieksekusi sebagai directed graph.

Komponen:

- Node registry.
- Graph validator.
- Dependency resolver.
- Execution planner.
- Job queue.
- Runtime executor.
- Result store.
- Error handler.

Alur eksekusi:

1. User klik **Run**.
2. Sistem validasi graph.
3. Sistem menghitung urutan eksekusi berdasarkan dependency.
4. Node dieksekusi sesuai urutan.
5. Status node diperbarui secara real-time.
6. Output node dikirim ke node berikutnya.
7. Preview/log ditampilkan.
8. Hasil akhir disimpan ke history/gallery.

### 5.2 Status Node

Setiap node memiliki status:

- Idle.
- Queued.
- Running.
- Success.
- Warning.
- Failed.
- Skipped.
- Cached.

### 5.3 Error Handling

Error harus mudah dipahami:

- Missing input.
- Type mismatch antar port.
- Model tidak tersedia.
- GPU/VRAM tidak cukup.
- Timeout.
- Provider API error.
- Invalid image/mask dimension.
- Node execution failed.

Setiap error perlu menampilkan:

- Node penyebab.
- Pesan ringkas.
- Detail teknis.
- Saran perbaikan.

---

## 6. Data Model Workflow

Workflow disimpan sebagai JSON.

Contoh struktur konseptual:

```json
{
  "version": "1.0",
  "name": "Basic Text to Image",
  "nodes": [
    {
      "id": "prompt_1",
      "type": "text_prompt",
      "position": { "x": 100, "y": 200 },
      "config": {
        "positive": "cinematic portrait, soft light",
        "negative": "blurry, low quality"
      }
    }
  ],
  "edges": [
    {
      "id": "edge_1",
      "source": "prompt_1",
      "sourcePort": "prompt",
      "target": "generate_1",
      "targetPort": "prompt"
    }
  ],
  "metadata": {
    "createdAt": "2026-01-01T00:00:00Z",
    "updatedAt": "2026-01-01T00:00:00Z"
  }
}
```

Hal yang perlu didukung:

- Versioning schema.
- Import/export workflow.
- Duplicate workflow.
- Workflow template.
- Workflow metadata.
- Compatibility migration.

---

## 7. UX / UI Requirements

### 7.1 Layout Utama

Rekomendasi layout:

- Kiri: node library/search.
- Tengah: canvas workflow.
- Kanan: inspector/config panel.
- Bawah: queue, logs, preview history.
- Atas: toolbar run/save/export/import.

### 7.2 Node Library

Node library harus punya:

- Search.
- Category.
- Recent nodes.
- Favorite nodes.
- Drag-to-canvas.
- Documentation tooltip.

Kategori awal:

- Input.
- Prompt.
- Model.
- Generation.
- Image Processing.
- Masking.
- Output.
- Utility.
- Advanced.

### 7.3 Inspector Panel

Saat node dipilih, panel kanan menampilkan:

- Nama node.
- Deskripsi.
- Input/output port.
- Field konfigurasi.
- Advanced config.
- Last execution result.
- Logs/error.
- Reset config.

### 7.4 Preview Experience

Preview gambar harus mendukung:

- Zoom.
- Pan.
- Before/after compare.
- Metadata view.
- Copy image.
- Save image.
- Open in gallery.
- Send as input to another workflow.

---

## 8. Backend / Architecture

### 8.1 Komponen Backend

Komponen yang disarankan:

1. **Workflow API**
   - CRUD workflow.
   - Import/export.
   - Template management.

2. **Node Registry**
   - Daftar node yang tersedia.
   - Schema input/output.
   - Default config.
   - Validation rules.

3. **Execution Service**
   - Validasi graph.
   - Build execution plan.
   - Run jobs.
   - Stream status/log.

4. **Queue Service**
   - Pending jobs.
   - Retry.
   - Cancel.
   - Priority.

5. **Asset Store**
   - Input images.
   - Output images.
   - Masks.
   - Intermediate results.
   - Metadata.

6. **Model Manager**
   - Available models.
   - Provider config.
   - Checkpoint/LoRA metadata.
   - Health check.

7. **Realtime Channel**
   - WebSocket/SSE untuk status execution dan preview.

### 8.2 Frontend State

Frontend perlu memisahkan:

- Canvas graph state.
- Unsaved draft state.
- Selected node state.
- Execution status state.
- Preview/history state.
- Node registry cache.

---

## 9. Integrasi Agent Smara

Fitur agent yang direkomendasikan:

1. **Generate workflow from prompt**
   - User: “buat workflow text-to-image dengan upscale dan face restore”.
   - Agent membuat node dan edge otomatis.

2. **Explain workflow**
   - Agent menjelaskan fungsi tiap node.

3. **Fix workflow**
   - Agent mendeteksi node tidak terhubung, type mismatch, missing model.

4. **Optimize workflow**
   - Agent menyarankan cache, resolusi, step, sampler, dan urutan node.

5. **Create template**
   - Agent menyimpan workflow sebagai template reusable.

6. **Run with parameters**
   - Agent menjalankan workflow dengan variasi prompt/seed.

---

## 10. Roadmap Milestone

## Phase 0 — Discovery & Technical Design

Status 2026-05-23: **done untuk dokumen/schema v1**.

Tujuan:

- Menentukan scope MVP.
- Mendesain schema workflow.
- Mendesain node registry.
- Memilih library canvas graph.
- Menentukan execution strategy.

Deliverables:

- PRD Image Flow.
- Workflow JSON schema v1.
- Node schema v1.
- Wireframe UI.
- Technical architecture doc.

Checklist:

- [x] Definisikan user journey utama.
- [x] Definisikan 8-10 node awal.
- [x] Definisikan tipe port awal: text, image, model, number, json.
- [x] Lengkapi tipe port lanjutan: mask dedicated, latent, boolean, seed, conditioning.
- [x] Definisikan validasi graph awal.
- [x] Definisikan format output asset final untuk gallery/history.

---

## Phase 1 — MVP Canvas & Workflow CRUD

Status 2026-05-23: **done untuk MVP**.

Tujuan:

Membuat canvas workflow yang bisa membuat node, menghubungkan node, menyimpan, dan membuka workflow.

Deliverables:

- Canvas editor dasar.
- Node library dasar.
- Inspector panel dasar.
- Save/load workflow.
- Import/export JSON.

Fitur:

- [x] Infinite canvas.
- [x] Add node dari library.
- [x] Drag node.
- [x] Connect ports.
- [x] Delete node/edge.
- [x] Select node.
- [x] Edit config node.
- [x] Save workflow.
- [x] Load workflow.
- [x] Export workflow JSON.
- [x] Import workflow JSON.

Acceptance criteria:

- User bisa membuat workflow text-to-image dummy.
- Workflow bisa disimpan dan dibuka ulang tanpa kehilangan posisi node dan config.
- Graph validator bisa mendeteksi koneksi invalid.

---

## Phase 2 — Execution Engine MVP

Status 2026-05-23: **done untuk MVP, hardening lanjut di Phase 6**.

Tujuan:

Workflow dapat dijalankan dengan node execution sederhana.

Deliverables:

- Graph validator.
- Execution planner.
- Job runner.
- Node status real-time.
- Log panel.
- Preview output.

Fitur:

- [x] Run workflow.
- [x] Cancel workflow request.
- [x] Show node status.
- [x] Show execution logs.
- [x] Store output image lewat output provider.
- [x] Basic error handling.
- [x] Queue single job.

Node MVP yang dieksekusi:

- [x] Text Prompt Node.
- [x] Model Loader Node.
- [x] Generate Image Node.
- [x] Image Preview Node.
- [x] Image Output Node.

Catatan lanjutan:

- [ ] Cancel belum memutus provider HTTP call yang sudah berjalan.
- [ ] Status realtime masih polling, belum WebSocket/SSE.
- [ ] Retry failed node belum ada.
- [ ] Output belum masuk gallery/history khusus Image Flow.

Acceptance criteria:

- User bisa menjalankan workflow text-to-image end-to-end.
- Status node berubah dari queued/running/success/failed.
- Output image tampil di preview dan tersimpan di history.

---

## Phase 3 — Image Editing & Advanced Nodes

Status 2026-05-23: **selesai untuk MVP editing / target Phase 3 saat ini**.

Tujuan:

Mendukung workflow image-to-image dan editing.

Deliverables:

- Image input.
- Mask input/editor.
- Image-to-image generation.
- Inpaint/outpaint.
- Upscale.
- Batch execution awal.

Fitur:

- [x] Image Input Node.
- [x] Mask Input Node.
- [x] Mask editor canvas sederhana.
- [x] Image-to-Image Node.
- [x] Inpaint Node.
- [x] Outpaint Node.
- [x] Upscale Node.
- [x] Batch Prompt Node.
- [x] Random Seed Node.
- [x] Compare preview.

Catatan lanjutan Phase 3:

- [x] Mask editor advanced MVP: paint, erase, undo/redo, feather/blur brush, opacity control, zoom, dan overlay preview di atas image input.
- [x] Inpaint/outpaint/upscale tersedia via capability `edit_image` provider.
- [x] Batch execution bisa run banyak prompt sekaligus dengan multi-output untuk text-to-image, image-to-image, inpaint, outpaint, dan upscale.

Catatan non-blocking: provider lokal khusus untuk inpaint/outpaint/upscale belum dipisah karena perlu keputusan model/engine lokal (mis. ComfyUI/local worker/upscaler CLI). Ini dipindahkan ke perluasan Phase 4/6, bukan blocker Phase 3 MVP.

Catatan non-blocking: provider lokal khusus untuk inpaint/outpaint/upscale belum dipisah karena perlu keputusan model/engine lokal (mis. ComfyUI/local worker/upscaler CLI). Ini dipindahkan ke perluasan Phase 4/6, bukan blocker Phase 3 MVP.

- User bisa upload gambar, memberi mask, lalu menjalankan inpaint.
- User bisa upscale output dari generation.
- User bisa membandingkan before/after.

---

## Phase 4 — Model & Asset Management

Status 2026-05-23: **selesai untuk MVP Model & Asset Management**.

Next recommended start: **Phase 5 — Agent Automation & Templates**.

Tujuan:

Membuat pengelolaan model, LoRA, checkpoint, dan asset lebih matang.

Deliverables:

- Model manager.
- Asset browser.
- Metadata image.
- Gallery/history.
- Provider configuration.

Fitur:

- [x] Daftar model tersedia.
- [x] Health check model/provider.
- [ ] LoRA selection. *(ditunda: butuh integrasi engine/model lokal seperti ComfyUI atau registry LoRA khusus)*
- [x] Asset gallery.
- [x] Image metadata viewer.
- [x] Reuse output as input.
- [x] Delete/archive output.
- [x] Search/filter asset.
- [x] Simpan metadata workflow bersama output image.
- [x] Asset browser untuk input/output/mask.
- [x] Tombol "use as input" dari preview/gallery ke Image Input.

Acceptance criteria:

- User bisa memilih model dari daftar.
- User bisa melihat output sebelumnya dan memakainya sebagai input node.
- Metadata workflow tersimpan bersama gambar output.

---

## Phase 5 — Agent Automation & Templates

Status 2026-05-24: **selesai untuk MVP agent automation/template termasuk UI panel**.

Tujuan:

Image Flow menjadi agent-friendly dan template-driven.

Deliverables:

- Generate workflow dari prompt.
- Workflow template gallery.
- Workflow lint/fix.
- Workflow optimization suggestions.
- Parameterized workflow run.

Fitur:

- [x] Agent create workflow.
- [x] Agent explain workflow.
- [x] Agent fix invalid graph.
- [x] Agent optimize settings.
- [x] Save as template. *(template builtin tersedia; save custom via endpoint workflow save)*
- [x] Run template with parameters.
- [ ] Share workflow. *(ditunda ke Phase 6/kolaborasi karena butuh format publik/sync)*
- [x] Template cepat untuk text-to-image, image edit, inpaint, outpaint, upscale, batch prompt.
- [x] Agent generate workflow dari prompt user.
- [x] Agent auto-fix missing/invalid connection.
- [x] UI panel Templates & Agent di halaman Image Flow.
- [x] UI load template, run template, create/lint/fix/optimize/explain workflow.
- [x] Auto-layout dasar lewat posisi template/generated workflow.
- [x] UI panel Templates & Agent di halaman Image Flow.
- [x] UI load template, run template, create/lint/fix/optimize/explain workflow.
- [x] Auto-layout dasar lewat posisi template/generated workflow.
- User bisa meminta agent membuat workflow lengkap dari instruksi natural language.
- Agent bisa memperbaiki workflow yang koneksinya salah.
- Workflow template bisa dijalankan ulang dengan prompt/seed berbeda.

---

## Phase 6 — Production Hardening

Status 2026-05-24: **berjalan — hardening inti sudah masuk**.

Tujuan:

Menjadikan Image Flow stabil, aman, dan siap dipakai intensif.

Deliverables:

- Multi-job queue.
- Retry policy.
- Caching.
- Permission/security.
- Performance optimization.
- Telemetry.

Fitur:

- [x] Multi-job queue. *(concurrency limit 2)*
- [x] Job priority. *(priority field + scheduler memilih priority tertinggi lalu FIFO)*
- [x] Retry failed node. *(retry job dari status failed/canceled)*
- [ ] Cache intermediate output.
- [x] Resource limit. *(max image dimension validation)*
- [x] Timeout policy. *(per-job context timeout 10 menit)*
- [x] Audit log. *(JSONL event untuk queued/success/failed/canceled/retry)*
- [x] Usage metrics. *(jobs/assets/archive/bytes/audit count)*
- [ ] Crash recovery.
- [x] WebSocket/SSE untuk status job. *(SSE endpoint `/api/image-flow/events`)*
- [x] Provider-call cancellation dengan context. *(context cancel diteruskan ke tool call; provider yang tidak support tetap best-effort)*
- [x] Cleanup asset sementara. *(cleanup archived assets by age + optional file delete)*
- [x] Test coverage lebih lengkap untuk schema, runner, upload, mask, dan UI state. *(tambahan test resource limit, cleanup, priority scheduler; UI build tervalidasi)*

Acceptance criteria:

- Workflow besar tidak membuat UI freeze.
- Job bisa dicancel dan diretry.
- Output sementara dapat dipulihkan setelah restart.

---

## 11. Node Schema v1

Setiap node perlu punya definisi seperti:

- `type`
- `displayName`
- `description`
- `category`
- `inputs`
- `outputs`
- `configSchema`
- `defaultConfig`
- `runtimeHandler`
- `validationRules`
- `uiHints`

Contoh konseptual:

```json
{
  "type": "text_prompt",
  "displayName": "Text Prompt",
  "category": "Prompt",
  "outputs": [
    { "name": "positive", "type": "text" },
    { "name": "negative", "type": "text" }
  ],
  "configSchema": {
    "positive": { "type": "string", "multiline": true },
    "negative": { "type": "string", "multiline": true }
  }
}
```

---

## 12. Port Types

Tipe port awal:

- `text`
- `image`
- `mask`
- `model`
- `latent`
- `number`
- `boolean`
- `json`
- `seed`
- `conditioning`

Aturan:

- Port hanya bisa terkoneksi jika tipe kompatibel.
- Satu output boleh ke banyak input.
- Input tertentu bisa mandatory atau optional.
- Input bisa punya default value dari config.

---

## 13. Testing Plan

### Unit Test

- Graph validator.
- Node schema validation.
- Execution planner.
- Port compatibility.
- Workflow import/export.
- Node config validation.

### Integration Test

- Save/load workflow.
- Run workflow dummy.
- Run workflow text-to-image.
- Error handling saat model tidak tersedia.
- Cancel job.
- Retry job.

### E2E Test

Scenario utama:

1. User membuat workflow baru.
2. User menambahkan prompt node.
3. User menambahkan model node.
4. User menambahkan generate node.
5. User menambahkan preview/output node.
6. User menghubungkan node.
7. User menjalankan workflow.
8. User melihat output.
9. User menyimpan workflow.
10. User membuka workflow kembali.

### Manual QA

- Canvas terasa responsif.
- Drag/connect node intuitif.
- Error mudah dipahami.
- Preview image tidak lambat.
- Workflow kompleks tetap mudah dinavigasi.

---

## 14. Risiko & Mitigasi

### Risiko 1: Scope terlalu besar

Mitigasi:

- Batasi MVP ke canvas + CRUD + text-to-image workflow.
- Advanced nodes masuk phase berikutnya.

### Risiko 2: Execution engine kompleks

Mitigasi:

- Mulai dari DAG sederhana.
- Hindari loop di v1.
- Tambahkan conditional/loop setelah engine stabil.

### Risiko 3: UI canvas berat

Mitigasi:

- Virtualization/render optimization.
- Debounce state update.
- Pisahkan draft state dan execution state.

### Risiko 4: Model/provider tidak konsisten

Mitigasi:

- Buat provider abstraction.
- Standardisasi output node.
- Health check model/provider.

### Risiko 5: File asset membengkak

Mitigasi:

- Asset retention policy.
- Thumbnail generation.
- Cleanup cache.
- Compression untuk preview.

### Risiko 6: Workflow sulit di-debug

Mitigasi:

- Per-node logs.
- Per-node preview.
- Explain graph.
- Agent-assisted fix.

---

## 15. Prioritas Implementasi

Urutan paling disarankan:

1. Workflow schema.
2. Node schema.
3. Canvas basic.
4. Node library.
5. Inspector panel.
6. Save/load workflow.
7. Graph validation.
8. Execution planner dummy.
9. Execution status UI.
10. Text-to-image integration.
11. Preview/history.
12. Image input/editing nodes.
13. Model manager.
14. Agent workflow generation.
15. Production hardening.

---

## 16. MVP Definition

MVP dianggap selesai jika:

- User bisa membuat workflow baru dari UI.
- User bisa menambahkan node dari library.
- User bisa menghubungkan node.
- User bisa mengedit config node.
- User bisa menyimpan dan membuka workflow.
- User bisa menjalankan minimal satu workflow text-to-image.
- User bisa melihat output image.
- User bisa melihat error saat workflow tidak valid.

---

## 17. Nice-to-Have Setelah MVP

- Workflow marketplace/template gallery.
- Visual diff antar workflow version.
- Collaboration/multiplayer editing.
- Cloud execution.
- Remote GPU worker.
- Custom plugin node.
- Python script node.
- API endpoint untuk menjalankan workflow.
- Batch generation spreadsheet-style.
- Prompt versioning.
- A/B comparison antar model.
- Cost estimation per run.

---

## 18. Catatan Implementasi Teknis

Jika frontend berbasis React, pertimbangkan:

- React Flow / XYFlow untuk node canvas.
- Zustand/Jotai untuk state ringan.
- TanStack Query untuk API state.
- WebSocket/SSE untuk execution update.
- Zod/JSON Schema untuk workflow validation.

Jika backend Go:

- Workflow service di internal package.
- JSON schema validation.
- Job queue sederhana terlebih dahulu.
- WebSocket/SSE endpoint untuk event streaming.
- File/object storage untuk image output.

Jika ingin kompatibilitas seperti ComfyUI:

- Pelajari format graph ComfyUI sebagai referensi.
- Buat importer opsional setelah schema internal stabil.
- Jangan jadikan format ComfyUI sebagai schema internal utama kecuali memang ingin kompatibel penuh.

---

## 19. Open Questions

- Apakah eksekusi image model akan lokal, remote, atau hybrid?
- Apakah Image Flow akan menggunakan model bawaan, API eksternal, atau integrasi dengan ComfyUI backend?
- Apakah workflow perlu kompatibel import/export dengan ComfyUI?
- Apakah v1 perlu mendukung GPU worker?
- Apakah perlu multi-user/collaboration?
- Apakah output image masuk ke galeri global Smara atau hanya history per workflow?

---

## 20. Next Action

Rekomendasi langkah berikutnya:

1. Buat PRD detail untuk MVP Image Flow.
2. Tentukan library canvas.
3. Buat workflow schema v1.
4. Buat prototype canvas dengan 5 node dummy.
5. Tambahkan save/load workflow.
6. Baru lanjut execution engine.
