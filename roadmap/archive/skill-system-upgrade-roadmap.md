# Roadmap Upgrade Skill System Smara

## 1. Ringkasan

Roadmap ini menyusun arah upgrade fitur Skill System Smara agar berkembang dari automation runner menjadi adaptive workflow system yang aman, mudah diaudit, dapat direkomendasikan secara cerdas, dan bisa diperbaiki melalui refine/enhance tanpa kehilangan lineage.

Fokus utama roadmap:

- Memperkuat kualitas skill melalui linting dan validasi.
- Membuat proses refine/enhance lebih aman melalui preview, diff, dan approval.
- Memanfaatkan lineage untuk history, compare, dan rollback.
- Meningkatkan discoverability melalui search, tree, graph, inspect, dan recommendation.
- Menambahkan metadata risiko dan approval gate untuk skill mutating, destructive, atau remote-write.
- Menyimpan run history lokal agar skill bisa dianalisis, diranking, dan diperbaiki berdasarkan data.

## Status Implementasi

Legend:

- [x] Selesai dan test PASS.
- [~] Sebagian sudah ada/fondasi tersedia, belum lengkap sesuai acceptance criteria.
- [ ] Belum diimplementasikan.

Checklist phase:

- [x] Phase 1 — Quality Gate Skill: command CLI `skill lint/validate`, validasi metadata/dependency/placeholder/tool-known/duplicate, dan test terkait sudah diverifikasi PASS.
- [x] Phase 2 — Safe Refine/Enhance Workflow: preview/diff/apply dengan proposal, auto-lint gate, lineage apply, dan test terkait sudah diverifikasi PASS.
- [x] Phase 3 — History, Compare, dan Rollback: command CLI `skill history`, `skill compare`, `skill rollback`, backend compare/snapshot resolver, dan test history/compare/rollback sudah diverifikasi PASS.
- [x] Phase 4 — Risk Metadata dan Approval Gate: metadata risiko di `Skill`, risk classifier, dry-run summary, dan approval gate `skill run --approve` sudah terimplementasi dan test PASS.
- [x] Phase 5 — UX Discoverability: command CLI `skill inspect`, `skill search --tag/--local`, dan `skill tree --format json` sudah tersedia/diverifikasi PASS.
- [x] Phase 6 — Run History dan Success Tracking: local tracker SQLite, command `skill runs/stats`, metadata status/failed_step/approval/version, dan test Phase 6 sudah diverifikasi PASS.
- [x] Phase 7 — Skill Recommendation dan Ranking: backend recommendation, ranking berbasis query/tag/tool/dependency/history/risk, CLI `skill recommend/suggest`, output text/json, dan test Phase 7 sudah diverifikasi PASS.
- [x] Phase 8 — Dependency Workflow Composition: typed dependency `requires/suggests/precheck/postcheck`, composition plan, precheck blocking, postcheck execution, suggestion non-blocking, dan test composition sudah diverifikasi PASS.
- [x] Phase 9 — Secure Skill Import/Install Review: review mode remote install, lint/risk scan, approval gate sebelum save, allow/blocklist tool, inventory operasi, dan test install review sudah diverifikasi PASS.

Catatan verifikasi terakhir: `go test ./internal/skill ./cmd/smara` PASS; Phase 1–9 sudah diverifikasi dari `cmd/smara/skill.go`, `cmd/smara/skill_phase6_test.go`, `cmd/smara/skill_phase7_test.go`, `internal/skill/lint.go`, `internal/skill/refinement_plan.go`, `internal/skill/history.go`, `internal/skill/risk.go`, `internal/skill/tracker.go`, `internal/skill/tree.go`, `internal/skill/recommendation.go`, `internal/skill/composition.go`, dan `internal/skill/install_review.go`.


Skill System yang dituju memiliki kemampuan berikut:

1. Skill dapat divalidasi otomatis sebelum disimpan, diimpor, atau dijalankan.
2. Perubahan hasil refine/enhance bisa dilihat dalam bentuk preview/diff sebelum apply.
3. Lineage skill dapat digunakan untuk history, compare, dan rollback.
4. User dapat menemukan skill dengan search, tag, tree, graph, dan recommendation.
5. Skill berisiko tinggi wajib melewati approval gate.
6. Sistem menyimpan run history lokal untuk mengetahui keberhasilan, kegagalan, durasi, dan step bermasalah.
7. Dependency skill dapat berkembang menjadi workflow composition dengan precheck dan postcheck.
8. Import/install skill dari sumber eksternal memiliki review mode yang aman.

## 4. Prioritas Implementasi

Urutan prioritas direkomendasikan:

1. Skill lint/validate.
2. Preview/diff untuk refine/enhance.
3. History, compare, dan rollback dari lineage.
4. Risk metadata dan approval gate.
5. Skill search, inspect, tree, dan graph UX.
6. Run history dan success tracking.
7. Recommendation/ranking skill.
8. Dependency workflow composition.
9. Security review untuk import/install skill.

## 5. Roadmap Bertahap

### Phase 1 — Quality Gate Skill

Tujuan: memastikan skill yang dibuat, di-refine, atau di-import memiliki struktur valid dan aman secara minimum.

Deliverable:

- Command/API `skill lint`.
- Validator metadata skill.
- Validator dependency graph.
- Validator placeholder parameter.
- Validator step kosong atau tool tidak dikenal.

Suggested CLI:

```bash
smara skill lint
smara skill lint <skill-name>
smara skill validate <skill-name>
```

Validasi minimum:

- Nama skill menggunakan kebab-case.
- Deskripsi tidak kosong dan cukup informatif.
- Step tidak kosong.
- Tool yang dipakai tersedia.
- Parameter required punya deskripsi.
- Placeholder di step sesuai parameter yang dideklarasikan.
- Dependency valid.
- Tidak ada circular dependency.
- Tidak ada duplicate skill name.

Acceptance criteria:

- Skill valid menghasilkan status PASS.
- Skill dengan dependency hilang menghasilkan error jelas.
- Skill dengan circular dependency menghasilkan error jelas.
- Skill dengan placeholder tidak dikenal menghasilkan error jelas.
- Test unit untuk validator PASS.

Testing:

- Unit test untuk valid skill.
- Unit test untuk invalid name.
- Unit test untuk missing dependency.
- Unit test untuk circular dependency.
- Unit test untuk unknown tool.
- Unit test untuk undeclared placeholder.

---

### Phase 2 — Safe Refine/Enhance Workflow

Tujuan: membuat refine/enhance transparan dan aman sebelum diterapkan.

Deliverable:

- Mode preview untuk refine/enhance.
- Diff skill lama vs skill baru.
- Ringkasan perubahan.
- Approval sebelum apply.
- Auto-lint hasil refine sebelum disimpan.

Suggested CLI:

```bash
smara skill refine <skill-name> --preview
smara skill refine <skill-name> --diff
smara skill refine <skill-name> --apply
smara skill enhance <skill-name> --preview
smara skill enhance <skill-name> --apply
```

Preview minimal menampilkan:

- Perubahan nama/deskripsi bila ada.
- Step yang ditambah.
- Step yang dihapus.
- Step yang berubah.
- Parameter baru/hilang.
- Dependency baru/hilang.
- Risk metadata baru/hilang.
- Hasil lint skill baru.

Acceptance criteria:

- Refine preview tidak mengubah file/data skill.
- Refine apply menyimpan lineage baru.
- Diff dapat menunjukkan perubahan step secara jelas.
- Hasil refine yang invalid tidak bisa di-apply tanpa override eksplisit.
- Test lineage tetap PASS setelah refine berulang.

Testing:

- Unit test preview tidak mutating.
- Unit test apply membuat lineage.
- Unit test diff step add/remove/update.
- Unit test invalid refined skill diblokir.

---

### Phase 3 — History, Compare, dan Rollback

Tujuan: memanfaatkan lineage yang sudah ada sebagai version history skill.

Deliverable:

- Command/API history skill.
- Compare antar versi skill.
- Rollback ke versi sebelumnya.
- Metadata versi: timestamp, parent version, reason, actor/source.

Suggested CLI:

```bash
smara skill history <skill-name>
smara skill compare <skill-name> --from v1 --to v3
smara skill rollback <skill-name> --to v2
```

Data history minimum:

- Version ID.
- Created at.
- Refined from.
- Change reason.
- Summary perubahan.
- Checksum/content hash.

Acceptance criteria:

- History menampilkan urutan versi dari awal sampai terbaru.
- Compare menunjukkan perubahan antar dua versi.
- Rollback menghasilkan versi baru yang menunjuk ke versi target, bukan menghapus history lama.
- Rollback dapat dibatalkan dengan rollback ke versi berikutnya.

Testing:

- Unit test history setelah multiple refine.
- Unit test compare antar versi.
- Unit test rollback membuat lineage baru.
- Unit test rollback ke versi tidak ada menghasilkan error jelas.

---

### Phase 4 — Risk Metadata dan Approval Gate

Tujuan: mencegah skill berisiko berjalan tanpa konfirmasi eksplisit.

Deliverable:

- Field metadata risiko pada skill.
- Risk classifier awal berdasarkan tool/command.
- Approval gate sebelum menjalankan skill berisiko.
- Dry-run/simulation summary untuk skill berisiko.

Metadata yang direkomendasikan:

```yaml
risk_level: low | medium | high
requires_approval: true
mutates_files: true
remote_write: false
destructive: false
uses_shell: true
uses_network: false
```

Aturan awal:

- `write_file`, `edit_file`, `delete_file` => mutates_files.
- `delete_file`, `rm`, `drop database`, `truncate` => destructive/high risk.
- SSH remote write/deploy => remote_write/high risk.
- Shell command tanpa allowlist => medium/high risk.
- Read-only tools => low risk.

Suggested CLI:

```bash
smara skill inspect <skill-name> --risk
smara skill run <skill-name> --dry-run
smara skill run <skill-name> --approve
```

Acceptance criteria:

- Skill read-only bisa berjalan tanpa approval tambahan.
- Skill mutating meminta approval.
- Skill destructive meminta approval eksplisit yang lebih kuat.
- Dry-run menampilkan tool/command/file/remote target yang akan dipakai.

Testing:

- Unit test risk classifier.
- Integration test skill low risk.
- Integration test skill mutating butuh approval.
- Integration test destructive skill diblokir tanpa approval.

---

### Phase 5 — UX Discoverability: Tree, Graph, Inspect, Search

Tujuan: membuat skill mudah ditemukan, dipahami, dan dikelola oleh user.

Deliverable:

- Command tree.
- Command graph.
- Command inspect.
- Command search.
- Filter berdasarkan tag/risk/unused/broken.

Suggested CLI:

```bash
smara skill tree
smara skill graph --format json
smara skill inspect <skill-name>
smara skill search "deploy node"
smara skill list --tag deploy
smara skill list --risk high
smara skill list --unused
smara skill list --broken
```

Output inspect minimal:

- Nama dan deskripsi.
- Tags/kategori.
- Parent/children.
- Dependencies.
- Risk metadata.
- Parameters.
- Steps summary.
- Last used.
- Success/failure count bila tersedia.
- Lineage latest version.

Acceptance criteria:

- User bisa melihat skill tree dengan parent-child jelas.
- User bisa export graph JSON.
- Search menemukan skill berdasarkan nama, deskripsi, tag, dan dependency.
- Inspect cukup informatif untuk memutuskan apakah skill aman dijalankan.

Testing:

- Unit test search by name.
- Unit test search by description.
- Unit test search by tag.
- Unit test tree output.
- Snapshot test untuk inspect output jika format stabil.

---

### Phase 6 — Run History dan Success Tracking

Tujuan: memberi observability lokal terhadap penggunaan skill.

Deliverable:

- Local run history store.
- Statistik pemakaian skill.
- Statistik keberhasilan/kegagalan.
- Durasi eksekusi.
- Last error dan failed step.

Metadata run yang direkomendasikan:

```yaml
run_id: string
skill_name: string
version_id: string
started_at: timestamp
finished_at: timestamp
status: success | failed | cancelled
failed_step: string | null
error_summary: string | null
duration_ms: number
approval_granted: boolean
```

Suggested CLI:

```bash
smara skill runs
smara skill runs <skill-name>
smara skill stats
smara skill stats <skill-name>
```

Acceptance criteria:

- Setiap run skill tercatat lokal.
- Run sukses/gagal tercatat dengan status benar.
- Error step tersimpan secara ringkas.
- Statistik bisa menampilkan skill yang sering gagal.

Testing:

- Unit test run success record.
- Unit test run failure record.
- Unit test stats aggregation.
- Integration test run skill menghasilkan log history.

---

### Phase 7 — Skill Recommendation dan Ranking

Tujuan: membuat Smara lebih pintar memilih atau menyarankan skill relevan.

Deliverable:

- Scoring skill berdasarkan query user.
- Ranking berdasarkan nama, deskripsi, tag, dependency, dan histori keberhasilan.
- Recommendation mode saat intent ambigu.
- Explanation mengapa skill direkomendasikan.

Suggested CLI/API:

```bash
smara skill recommend "deploy aplikasi node ke vps"
smara skill suggest "cek server"
```

Scoring awal:

- Keyword match pada name.
- Keyword match pada description.
- Tag match.
- Tool match.
- Historical success rate.
- Recency usage.
- Penalty untuk high-risk skill jika query tidak eksplisit.

Acceptance criteria:

- Query deploy memunculkan skill deploy di ranking atas.
- Query monitoring memunculkan skill monitoring di ranking atas.
- Sistem dapat menjelaskan alasan rekomendasi.
- Jika confidence rendah, Smara meminta klarifikasi, bukan menjalankan otomatis.

Testing:

- Unit test ranking by name.
- Unit test ranking by tag.
- Unit test ranking dengan success rate.
- Unit test low confidence menghasilkan ask/clarify.

---

### Phase 8 — Dependency Workflow Composition

Tujuan: mengubah dependency skill dari sekadar relasi graph menjadi workflow composition yang dapat menjalankan precheck dan postcheck.

Deliverable:

- Jenis dependency baru: requires, suggests, precheck, postcheck.
- Runner yang dapat menjalankan precheck sebelum skill utama.
- Runner yang dapat menjalankan postcheck setelah skill utama.
- Policy ketika precheck gagal.

Format metadata yang direkomendasikan:

```yaml
dependencies:
  requires:
    - repo-quick-analysis
  suggests:
    - frontend-lint-test-build
  precheck:
    - vps-health-check
  postcheck:
    - vps-pm2-diagnose
```

Acceptance criteria:

- Precheck dijalankan sebelum skill utama.
- Skill utama tidak berjalan jika required precheck gagal.
- Suggestion tidak memblokir eksekusi.
- Postcheck berjalan setelah skill utama jika dikonfigurasi.
- Execution plan ditampilkan sebelum run untuk workflow kompleks.

Testing:

- Unit test dependency type parsing.
- Integration test precheck success.
- Integration test precheck failure blocks main skill.
- Integration test postcheck runs after main.

---

### Phase 9 — Secure Skill Import/Install Review

Tujuan: memastikan skill dari path lokal, GitHub, raw URL, atau sumber eksternal tidak langsung membahayakan environment user.

Deliverable:

- Review mode default untuk import remote.
- Lint otomatis setelah import.
- Risk scan sebelum install final.
- Approval sebelum menyimpan skill remote.
- Blocklist/allowlist tool berbahaya.

Suggested CLI:

```bash
smara skill install <source> --review
smara skill install <source> --approve
smara skill import <path> --lint
```

Review import minimal menampilkan:

- Nama skill.
- Deskripsi.
- Source.
- Tools yang digunakan.
- Command shell yang akan dijalankan.
- File yang akan ditulis/dihapus.
- Remote host/network yang disentuh.
- Risk level.
- Hasil lint.

Acceptance criteria:

- Skill dari remote tidak auto-run setelah install.
- Skill remote high-risk butuh approval eksplisit.
- Skill invalid tidak bisa diinstall tanpa override eksplisit.
- User dapat membatalkan install setelah review.

Testing:

- Unit test import valid.
- Unit test import invalid.
- Unit test remote high-risk requires approval.
- Integration test install review tidak menjalankan step skill.

## 6. Suggested Data Model Additions

### Skill Metadata

```yaml
name: deploy-node-pm2-git-pull
description: Deploy aplikasi Node.js ke VPS via git pull, install dependency, build jika ada, dan reload PM2.
tags:
  - deploy
  - nodejs
  - pm2
risk_level: high
requires_approval: true
mutates_files: true
remote_write: true
destructive: false
uses_shell: true
uses_network: true
version_id: v3
lineage:
  parent_version_id: v2
  created_at: "2026-05-25T00:00:00Z"
  reason: "Add PM2 postcheck and build detection"
```

### Run History

```yaml
run_id: run_20260525_001
skill_name: deploy-node-pm2-git-pull
version_id: v3
started_at: "2026-05-25T10:00:00Z"
finished_at: "2026-05-25T10:01:20Z"
status: success
failed_step: null
error_summary: null
duration_ms: 80000
approval_granted: true
```

## 7. End-to-End Verification Plan

End-to-end scenario yang perlu divalidasi setelah roadmap diimplementasikan:

1. Buat skill baru.
2. Jalankan `skill lint` dan pastikan PASS.
3. Refine skill dengan mode preview.
4. Lihat diff skill lama vs skill baru.
5. Apply refine setelah approval.
6. Cek history skill.
7. Compare versi lama dan baru.
8. Rollback ke versi lama.
9. Jalankan skill low-risk tanpa approval tambahan.
10. Jalankan skill high-risk dan pastikan approval gate muncul.
11. Jalankan skill sampai sukses dan pastikan run history tercatat.
12. Jalankan search/recommend dan pastikan skill relevan muncul.
13. Import skill remote high-risk dan pastikan review mode aktif.

## 8. Risks and Mitigations

| Risiko | Dampak | Mitigasi |
| --- | --- | --- |
| Refine menghasilkan skill invalid | Skill gagal dijalankan | Auto-lint sebelum apply |
| Rollback menghapus history | Kehilangan audit trail | Rollback harus membuat versi baru, bukan overwrite history |
| Risk classifier terlalu agresif | UX terganggu banyak approval | Sediakan override/config policy |
| Risk classifier terlalu longgar | Operasi berbahaya bisa lolos | Default konservatif untuk shell/remote/destructive |
| Search/recommend salah memilih skill | Skill tidak relevan dijalankan | Jika confidence rendah, minta klarifikasi |
| Run history menyimpan data sensitif | Potensi leak lokal | Redact secret/token/password dari log |
| Import remote berbahaya | Eksekusi command tak aman | Review mode default dan larangan auto-run |
| Dependency workflow terlalu kompleks | Sulit debug | Tampilkan execution plan sebelum run |

## 9. Rollback Strategy Implementasi

Jika implementasi upgrade bermasalah:

1. Feature flag setiap fitur baru:
   - `skill_lint_enabled`
   - `skill_refine_preview_enabled`
   - `skill_risk_gate_enabled`
   - `skill_run_history_enabled`
   - `skill_recommendation_enabled`
2. Pertahankan compatibility dengan format skill lama.
3. Migrasi metadata harus additive, bukan breaking.
4. Jika run history bermasalah, matikan pencatatan tanpa mematikan runner skill.
5. Jika risk gate false positive, izinkan bypass eksplisit dengan approval manual.

## 10. Definition of Done Global

Roadmap dianggap selesai bila:

- Semua skill dapat dilint dan divalidasi.
- Refine/enhance mendukung preview, diff, apply, dan lineage.
- Skill history, compare, dan rollback tersedia.
- Risk metadata diterapkan pada skill dan approval gate aktif.
- Tree, graph, inspect, search, dan list filter tersedia.
- Run history lokal mencatat success/failure/duration/error.
- Recommendation dapat menyarankan skill relevan dengan alasan.
- Dependency precheck/postcheck dapat menjalankan workflow komposit.
- Import/install skill eksternal aman melalui review mode.
- Test unit dan integration untuk fitur utama PASS.

## 11. Milestone Rekomendasi

### Milestone A — Safety and Quality Foundation

Scope:

- Phase 1: Skill lint/validate.
- Phase 2: Safe refine/enhance preview/diff.
- Phase 4: Risk metadata dasar.

Outcome:

Skill lebih aman dibuat, diubah, dan dijalankan.

### Milestone B — Versioning and UX Management

Scope:

- Phase 3: History/compare/rollback.
- Phase 5: Tree/graph/inspect/search.

Outcome:

Skill mudah dikelola, diaudit, dan dikembalikan jika rusak.

### Milestone C — Observability and Intelligence

Scope:

- Phase 6: Run history.
- Phase 7: Recommendation/ranking.

Outcome:

Smara bisa belajar dari penggunaan lokal dan menyarankan skill lebih relevan.

### Milestone D — Advanced Workflow and Secure Import

Scope:

- Phase 8: Dependency workflow composition.
- Phase 9: Secure import/install review.

Outcome:

Skill menjadi workflow adaptif yang tetap aman saat digunakan atau diimpor dari sumber eksternal.
