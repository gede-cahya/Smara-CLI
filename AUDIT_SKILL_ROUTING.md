# Audit Routing Skill Smara

Tanggal: 2026-06-02

## Ringkasan

Status umum: **fungsi skill core berjalan**. `skill_list`, `skill_create`, `skill_run`, dan recommendation engine tersedia dan test relevan PASS.

Namun auto-routing skill masih **prompt-driven**, bukan deterministic pre-router. Artinya agent diberi daftar skill dan instruksi untuk memakai `skill_run`, tetapi belum ada lapisan kode yang otomatis menghitung rekomendasi skill dari prompt user lalu menginjeksi Top-N skill relevan ke konteks setiap request.

## Bukti Kode

- `internal/agent/skill_context.go`
  - Menginjeksi daftar skill tersimpan ke system prompt.
  - Menginstruksikan LLM: jika ada skill relevan, prioritaskan `skill_run`.
  - Workflow mode sengaja mematikan auto-create/tool umum.

- `internal/skill/recommendation.go`
  - Ada scoring skill berdasarkan nama, deskripsi, tag, dependency, tool, histori sukses, dan penalti risk.

- `cmd/smara/skill.go`
  - `skill recommend <query>` sudah memakai `skill.RecommendSkills(...)`.

- `internal/agent/supervisor.go`
  - `skill_run` dieksekusi dengan `skill.Load(name)` lalu `sk.Run(...)`.

## Hasil Test

Command:

```bash
go test ./internal/skill ./internal/agent -run 'Skill|Recommend|Auto'
```

Hasil: **PASS**

## Temuan

1. **PASS — Skill registry tersedia**
   - Agent context bisa menampilkan skill tersimpan.

2. **PASS — Skill runner tersedia**
   - Tool `skill_run` registered dan bisa menjalankan skill.

3. **PASS — Recommendation engine tersedia**
   - Ranking berdasarkan query sudah ada.

4. **GAP — Recommendation belum otomatis dipakai oleh agent routing**
   - `RecommendSkills(...)` hanya ditemukan di CLI command `skill recommend`, belum di agent request path.
   - Dampaknya: agent bisa memilih skill otomatis via instruksi prompt, tetapi tidak dibantu Top-N rekomendasi yang deterministic.

5. **GAP — Confidence policy belum jadi guardrail agent**
   - Recommendation punya `Clarify` dan `Confidence`, tetapi belum mengatur kapan agent harus bertanya klarifikasi sebelum `skill_run`.

6. **AMAN — Workflow mode tidak auto-create skill**
   - Sesuai aturan agar workflow tersimpan tidak menjalankan auto skill sembarangan.

## Rekomendasi Fix Prioritas

1. Tambahkan fungsi agent-side `buildSkillRecommendationContext(query string)` yang:
   - load semua skill lokal,
   - panggil `skill.RecommendSkills(query, skills, Limit: 5, StatsProvider: tracker jika tersedia)`,
   - injeksikan daftar Top-N ke prompt request.

2. Tambahkan instruksi eksplisit:
   - Jika rekomendasi confidence `high` dan task bukan chat sederhana, panggil `skill_run` dulu.
   - Jika confidence `medium`, boleh panggil skill jika cocok jelas.
   - Jika semua `Clarify=true`, tanya klarifikasi singkat.

3. Tambahkan test agent context:
   - query `cek vps` harus merekomendasikan `vps-health-check` bila skill tersedia.
   - query `deploy node app ke vps` harus merekomendasikan `deploy-node-pm2-git-pull`.
   - sapaan `halo` tidak boleh memaksa skill.

## Kesimpulan

Auto skill Smara **berfungsi**, tetapi kualitas pemanggilan otomatis bisa ditingkatkan signifikan dengan menghubungkan recommendation engine ke konteks agent runtime. Saat ini routing skill masih bergantung pada kemampuan LLM membaca daftar skill dan mengikuti instruksi.
