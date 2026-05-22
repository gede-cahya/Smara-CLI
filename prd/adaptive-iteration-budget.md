# PRD: Adaptive Iteration Budget Hard Cap

## 1. Ringkasan

Perubahan ini memperbaiki mekanisme adaptive iteration budget di Smara agar `agent_max_iterations` atau user/config override diperlakukan sebagai **base budget**, bukan sebagai **hard cap final**.

Sebelumnya, ketika user menetapkan budget misalnya `80`, sistem mengatur:

```text
base = 80
current = 80
hardCap = 80
```

Akibatnya fitur adaptive budget seperti `request_iteration_budget` tidak punya ruang tambahan karena `current` sudah sama dengan `hardCap`.

Setelah perubahan, sistem menggunakan policy hard cap adaptif berdasarkan mode kerja:

```text
base = 80
current = 80
hardCap = 160 atau 240 tergantung mode
```

Dengan begitu agent tetap mulai dari budget yang terkendali, tetapi bisa meminta tambahan secara aman selama masih berada di bawah batas global.

---

## 2. Latar Belakang

Smara memiliki mekanisme iteration budget untuk membatasi jumlah tool-call/step dalam satu turn. Mekanisme ini penting untuk:

- menghindari infinite loop,
- mengontrol biaya API,
- menjaga durasi eksekusi tetap wajar,
- mencegah agent terlalu agresif dalam satu turn.

Namun ditemukan masalah pada implementasi override budget. Ketika user atau config menetapkan `agent_max_iterations`, nilai tersebut digunakan sekaligus sebagai base, current, dan hard cap.

Contoh sebelum perubahan:

```text
Mode: plan
Base budget: 80
Active limit: 80
Hard cap: 80
Headroom: 0
```

Dalam kondisi tersebut, adaptive iteration budget secara praktis tidak berguna karena agent tidak bisa meminta tambahan.

---

## 3. Tujuan

### 3.1 Tujuan Utama

Mengaktifkan adaptive iteration budget secara efektif dengan memisahkan:

- **base budget**: jatah awal agent,
- **current limit**: batas aktif saat ini,
- **hard cap**: batas maksimal yang boleh dicapai setelah extension.

### 3.2 Tujuan Produk

- Agent dapat menyelesaikan task panjang dengan lebih stabil.
- Task besar dapat dilanjutkan tanpa terlalu sering terputus karena budget habis.
- Sistem tetap aman karena ada global hard cap.
- User tetap punya kontrol melalui mode kerja dan konfigurasi.

---

## 4. Non-Goals

Perubahan ini tidak bertujuan untuk:

- memberikan agent kemampuan menaikkan hard cap tanpa batas,
- menghapus sistem limit iteration,
- mengubah seluruh arsitektur supervisor agent,
- membuat budget ditentukan penuh oleh model AI,
- menaikkan semua mode menjadi heavy secara default.

---

## 5. Problem Statement

### 5.1 Masalah Sebelumnya

Kode lama memperlakukan user override sebagai hard cap final:

```go
if userMax > 0 {
    b.base = userMax
    b.hardCap = userMax
    b.current = userMax
}
```

Dampaknya:

```text
request_iteration_budget tidak bisa grant tambahan
```

karena:

```text
current == hardCap
```

### 5.2 Dampak ke User

User mengalami kasus seperti:

```text
batas tool sudah habis
adaptive budget tidak bisa bertambah
hard cap = active limit
```

Padahal task masih valid untuk dilanjutkan, misalnya:

- coding multi-file,
- debugging panjang,
- deploy server,
- integrasi fitur,
- refactor besar.

---

## 6. Solusi

### 6.1 Prinsip Solusi

User/config override tetap dihormati sebagai jatah awal, tetapi hard cap dihitung secara adaptif berdasarkan mode.

Formula umum:

```text
hardCap = min(base * multiplier(mode), globalMax)
```

Dengan guard:

```text
hardCap >= base
```

### 6.2 Global Cap

Ditambahkan batas global:

```go
AbsoluteIterationHardCap = 240
```

Tujuannya agar agent tidak bisa memperpanjang eksekusi tanpa batas.

### 6.3 Mode dan Multiplier

| Mode | Complexity | Multiplier | Contoh base 80 | Hard cap |
|---|---:|---:|---:|---:|
| ASK | light | 1x | 80 | 80 |
| RUSH | normal | 2x | 80 | 160 |
| PLAN | normal | 2x | 80 | 160 |
| TEST | normal | 2x | 80 | 160 |
| WORKFLOW | heavy | 3x | 80 | 240 |

---

## 7. Functional Requirements

### FR-1: Override diperlakukan sebagai base budget

Ketika user/config memberi nilai override, sistem harus menetapkan:

```text
base = override
current = override
```

### FR-2: Hard cap dihitung adaptif

Untuk override aktif, sistem harus menghitung hard cap berdasarkan mode:

```text
hardCap = computeAdaptiveHardCap(base, mode)
```

### FR-3: Mode ASK tetap ringan

Mode ASK tidak boleh otomatis mendapat multiplier besar.

Expected:

```text
ASK base 80 -> hardCap 80
```

### FR-4: Mode PLAN/RUSH/TEST mendapat normal multiplier

Expected:

```text
PLAN base 80 -> hardCap 160
RUSH base 80 -> hardCap 160
TEST base 80 -> hardCap 160
```

### FR-5: Mode WORKFLOW mendapat heavy multiplier

Expected:

```text
WORKFLOW base 80 -> hardCap 240
```

### FR-6: Hard cap tidak boleh melewati global cap

Expected:

```text
WORKFLOW base 120 -> raw 360 -> hardCap 240
```

### FR-7: Manual extension tetap dibatasi

Sistem `request_iteration_budget` tetap harus menghormati:

- hard cap,
- max manual extension,
- stuck-loop protection,
- safety multiplier yang sudah ada.

---

## 8. Non-Functional Requirements

### NFR-1: Safety

Agent tidak boleh bisa menaikkan hard cap secara absolut tanpa policy backend.

### NFR-2: Predictability

Mapping mode ke multiplier harus jelas dan mudah dipahami.

### NFR-3: Backward Compatibility

Perubahan tidak boleh merusak flow agent existing. Mode tanpa override tetap mengikuti default existing, kecuali memang dihitung melalui helper baru.

### NFR-4: Test Coverage

Perubahan harus dilengkapi unit test untuk mode utama dan clamp global.

---

## 9. User Experience

### Sebelum

User melihat status seperti:

```text
Mode: plan
Base budget: 80
Active limit: 80
Hard cap: 80
```

Jika agent meminta tambahan:

```text
denied: no headroom
```

### Sesudah

Untuk mode PLAN:

```text
Mode: plan
Base budget: 80
Active limit: 80
Hard cap: 160
```

Jika agent meminta tambahan 40:

```text
granted, active limit menjadi 120
```

Selama belum melewati hard cap.

---

## 10. Acceptance Criteria

Perubahan dianggap selesai jika:

- [x] User override tidak lagi membuat `hardCap == base` untuk mode normal/heavy.
- [x] Mode ASK tetap light dengan hard cap sama seperti base.
- [x] Mode PLAN menghasilkan hard cap 2x base, maksimal 240.
- [x] Mode RUSH menghasilkan hard cap 2x base, maksimal 240.
- [x] Mode TEST menghasilkan hard cap 2x base, maksimal 240.
- [x] Mode WORKFLOW menghasilkan hard cap 3x base, maksimal 240.
- [x] Unit test untuk adaptive hard cap tersedia.
- [x] Full test suite lolos.

---

## 11. File yang Diubah

```text
internal/agent/iteration_budget.go
internal/agent/iteration_budget_test.go
```

---

## 12. Test Plan

### 12.1 Unit Test

Jalankan:

```bash
go test ./internal/agent
```

Test yang perlu mencakup:

- ASK override tetap light,
- PLAN override menjadi 2x,
- RUSH override menjadi 2x,
- TEST override menjadi 2x,
- WORKFLOW override menjadi 3x,
- hard cap diclamp ke 240.

### 12.2 Regression Test

Jalankan:

```bash
go test ./...
```

Expected:

```text
all tests pass
```

---

## 13. Risiko

### Risiko 1: Agent berjalan lebih lama

Karena hard cap lebih besar, agent bisa melakukan lebih banyak tool-call dalam satu turn.

Mitigasi:

- global cap 240,
- max manual extension tetap aktif,
- stuck-loop detection tetap aktif.

### Risiko 2: Biaya API meningkat

Lebih banyak iteration dapat meningkatkan token/tool usage.

Mitigasi:

- ASK tetap light,
- heavy hanya untuk WORKFLOW,
- user tetap bisa membatasi base budget.

### Risiko 3: UX terasa lama untuk task besar

Task panjang mungkin berjalan lebih lama sebelum berhenti.

Mitigasi:

- agent diarahkan membuat checkpoint per tahap,
- task besar dipecah menjadi beberapa fase.

---

## 14. Rollback Plan

Jika perubahan perlu dibatalkan, rollback logic override menjadi:

```go
if userMax > 0 {
    b.base = userMax
    b.hardCap = userMax
    b.current = userMax
}
```

Atau opsi lebih aman:

```go
AbsoluteIterationHardCap = 160
```

untuk mengurangi ruang extension tanpa menghapus adaptive policy sepenuhnya.

---

## 15. Future Improvements

Beberapa pengembangan lanjutan yang bisa dipertimbangkan:

1. Tambah config eksplisit:

```toml
iteration_budget_mode = "normal"
iteration_budget_global_cap = 240
```

2. Tambah task classifier ringan:

```text
jawaban biasa -> light
coding multi-file -> normal
refactor besar -> heavy
deploy/server -> normal/heavy
```

3. Tambah status report yang lebih jelas:

```text
base: 80
current: 120
hardCap: 160
manualExtensions: 1/5
reason: multi-file refactor
```

4. Tambah checkpoint otomatis sebelum budget tersisa rendah.

---

## 16. Status

Status: Implemented

Perubahan sudah diterapkan dan diverifikasi dengan:

```bash
go test ./internal/agent ./internal/config ./cmd/smara
go test ./...
```

Hasil: pass.
