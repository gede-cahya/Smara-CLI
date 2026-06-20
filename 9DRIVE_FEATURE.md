# Fitur 9drive Integration - Smara CLI

## Ringkasan

Integrasi 9drive cloud storage ke Smara CLI untuk upload file ke cloud dengan API key authentication.

**Status**: ✅ Selesai dan sudah di-build ke binary `smara`

---

## Komponen yang Dibuat

### 1. **Client Library** (`internal/ninedrive/ninedrive.go`)
- Fungsi `NewClient(baseURL, apiKey)` untuk membuat client
- Fungsi `UploadFile(filePath, mimeType)` untuk upload single file
- Otomatis handle protocol 9drive (filesMeta → file field order)
- Support MIME type custom atau auto-detect

**Test**: `internal/ninedrive/ninedrive_test.go` ✅ PASS

### 2. **Config Integration** (`internal/config/config.go`)
```yaml
# ~/.smara/config.yaml
ninedrive_enabled: true
ninedrive_base_url: "http://localhost:4000"
ninedrive_api_key: "9d_live_Blak5Re21CMSIE_EHK0y_xApsbMZyGJnIvDzcp8wOw4"
```

### 3. **CLI Command** (`cmd/smara/ninedrive.go`)

#### Upload single file:
```bash
smara 9drive upload /path/to/file.jpg
```

#### Upload multiple files:
```bash
smara 9drive upload file1.jpg file2.png file3.pdf
```

#### Upload dengan wildcard:
```bash
smara 9drive upload /path/to/images/*.jpg
```

#### Upload dengan custom MIME type:
```bash
smara 9drive upload --mime-type "application/pdf" document.pdf
```

### 4. **Agent Tool** (`internal/agent/ninedrive_tool.go`)

Tool `upload_to_9drive` tersedia untuk agent dengan parameter:
- `file_path` (required): Path ke file yang akan diupload
- `mime_type` (optional): MIME type override

**Contoh penggunaan agent**:
```
User: "Upload gambar hasil generate ke 9drive"
Agent: [memanggil generate_image] → [memanggil upload_to_9drive]
```

### 5. **Web UI Settings** (`web/src/pages/Config.tsx`)

Section "9drive Cloud Storage" dengan fields:
- **Enable 9drive**: Toggle on/off
- **Base URL**: URL server 9drive (default: http://localhost:4000)
- **API Key**: Input password untuk API key

---

## Cara Penggunaan

### Setup Awal

1. **Enable 9drive di config**:
```bash
smara config set ninedrive_enabled true
```

2. **Set base URL** (jika server tidak di localhost):
```bash
smara config set ninedrive_base_url "http://your-server:4000"
```

3. **Set API key**:
```bash
smara config set ninedrive_api_key "9d_live_Blak5Re21CMSIE_EHK0y_xApsbMZyGJnIvDzcp8wOw4"
```

### Penggunaan CLI

```bash
# Upload foto
smara 9drive upload ~/Pictures/vacation.jpg

# Upload semua file di folder
smara 9drive upload ~/Documents/*.pdf

# Upload dengan verbose output
smara 9drive upload -v large_file.zip
```

### Penggunaan Agent

Dalam percakapan dengan agent:
```
User: "Generate logo untuk project baru dan upload ke 9drive"
Agent: 
  1. [generate_image] → Membuat logo
  2. [upload_to_9drive] → Upload ke cloud
  3. Response: "✓ Logo berhasil diupload ke 9drive"
```

### Web UI

1. Buka Smara Web
2. Klik Settings (ikon gear)
3. Scroll ke section "9drive Cloud Storage"
4. Toggle "Enable 9drive" ke ON
5. Isi Base URL dan API Key
6. Klik Save

---

## Contoh Output

### CLI Success:
```
$ smara 9drive upload test.jpg
{"success":true,"file_id":"abc123","url":"https://9drive.cloud/f/abc123"}
✓ Uploaded test.jpg (45231 bytes)
```

### CLI Error (file tidak ada):
```
$ smara 9drive upload missing.jpg
Error: File tidak ada atau tidak bisa dibaca: missing.jpg
```

### Agent Success:
```
Agent: [upload_to_9drive]
- file_path: "/tmp/generated_logo.png"
- mime_type: "image/png"

Response: 
{"success":true,"file_id":"xyz789","url":"https://9drive.cloud/f/xyz789"}
✓ Logo berhasil diupload ke 9drive
```

---

## Testing

```bash
# Test client library
go test ./internal/ninedrive/... -v

# Test CLI command
./smara 9drive upload test_file.txt

# Test dengan verbose
./smara 9drive upload -v test_file.txt
```

---

## Troubleshooting

### Error: "9drive is not enabled"
**Solusi**: Enable di config
```bash
smara config set ninedrive_enabled true
```

### Error: "Failed to upload"
**Kemungkinan penyebab**:
- Server 9drive tidak running
- API key salah
- Network issue

**Solusi**:
1. Cek server status
2. Verify API key di config
3. Test dengan `-v` flag untuk detail error

### Error: "File tidak ada"
**Solusi**: Pastikan path file benar dan file exists
```bash
ls -la /path/to/file.jpg
```

---

## Files yang Dibuat/Dimodifikasi

### Created:
- `internal/ninedrive/ninedrive.go` (client library)
- `internal/ninedrive/ninedrive_test.go` (tests)
- `cmd/smara/ninedrive.go` (CLI command)
- `internal/agent/ninedrive_tool.go` (agent tool)

### Modified:
- `internal/config/config.go` (tambah 3 config fields)
- `internal/agent/builtin_tools.go` (register tool)
- `web/src/pages/Config.tsx` (tambah UI settings)

---

## API Protocol

9drive menggunakan multipart/form-data dengan urutan khusus:
1. `filesMeta` field (JSON dengan sizeBytes)
2. `file` field (binary content)

Client library sudah handle otomatis.

---

## Next Steps (Optional)

Jika ingin enhance fitur:
- [ ] Upload multiple files sekaligus
- [ ] Download file dari 9drive
- [ ] List files di 9drive account
- [ ] Delete file dari 9drive
- [ ] Integration dengan Smara memory (backup otomatis)
- [ ] Upload screenshot dari agent workflow

---

## Catatan Teknis

- **Timeout**: 120 detik untuk upload (bisa disesuaikan di client)
- **Max file size**: Tergantung server 9drive config
- **MIME types**: Support semua standard MIME types
- **Security**: API key disimpan di config file (chmod 600 recommended)

---

**Tanggal**: 2026-06-20  
**Status**: ✅ Production Ready  
**Binary**: `./smara` (already built)
