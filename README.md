# nakama

Several interactive `claude` (Claude Code) sessions in one terminal, with an optional pixel-art
"office" where every session is a character that walks, works and sleeps. *(The interface is in
Indonesian.)*

Beberapa sesi Claude Code interaktif dalam satu terminal: daftar sesi di semua folder, status dan
pemakaian konteks, serta "kantor" bergambar tempat tiap sesi menjadi karakter.

> Proyek pribadi, tidak resmi, dan tidak berafiliasi dengan Anthropic.

## Fitur

- Tiga panel: sesi aktif, pohon folder/sesi, dan chat (claude asli lewat PTY).
- Membuka sesi baru atau melanjutkan sesi lama dari semua folder; beberapa akun (`~/.claude`, `~/.claude-<nama>`).
- Status tiap sesi (menunggu, bekerja, idle), subagen, konteks, token hari ini, dan sisa kuota.
- Notifikasi saat sesi menunggumu atau selesai (bel terminal; notifikasi Windows bila di WSL).
- Kantor (`v`): karakter bekerja di tempatnya, tidur di kamar istirahat, dan mendatangi sesi yang dikirimi pesan. Klik karakter untuk membuka chatnya.

## Kebutuhan

- Linux atau WSL2. Hanya diuji di WSL2; sebagian fitur memakai `/proc`, jadi macOS belum didukung.
- [Go](https://go.dev/dl/) 1.26 atau lebih baru.
- [Claude Code](https://claude.com/claude-code) (`claude`) sudah terpasang dan login.
- Untuk kantor bergambar: terminal yang mendukung **sixel** (mis. Windows Terminal 1.22+, WezTerm, foot). Tanpa itu, tiga panel tetap jalan.

## Instalasi

```sh
go install github.com/kalyskhairy/nakama/cmd/nakama@latest
```

Binernya ada di `$(go env GOPATH)/bin`. Pastikan folder itu ada di `PATH`:

```sh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc && source ~/.bashrc
```

Dari kode sumber:

```sh
git clone https://github.com/kalyskhairy/nakama.git
cd nakama
go install ./cmd/nakama
```

Lalu jalankan `nakama` dari folder mana pun. Pertama kali, pilih folder yang mau ditampilkan.

## Tombol utama

| Tombol | Fungsi |
|---|---|
| `↑` `↓` / `j` `k` | pilih baris |
| `Tab` | pindah panel |
| `Enter` | buka sesi / chat |
| `Ctrl+Space` | pindah dari chat ke daftar |
| `n` | sesi baru |
| `f` | pilih folder |
| `z` | chat layar penuh |
| `v` | kantor |
| `x` | tutup sesi yang sudah selesai |
| `F1` / `?` | bantuan lengkap |
| `q` | keluar (**semua sesi yang dibuka nakama ikut berhenti**; bisa dilanjutkan lagi) |

## Opsi

| Opsi | Arti |
|---|---|
| `--theme <nama>` | tema kantor (bawaan `default`) |
| `--graphics auto\|sixel\|off` | gambar kantor |
| `--font 10x20` | ukuran sel (piksel) bila terminal tidak melaporkannya |
| `--notify=false` | matikan notifikasi |
| `--offline` | jangan pernah mengambil sisa kuota dari jaringan |
| `--prefix 0x1d` | ganti `Ctrl+Space` (di sini `Ctrl+]`) |
| `--context-window 200000` | untuk persentase konteks |

## Privasi dan hal yang perlu diketahui

- nakama **membaca** `~/.claude*`: transkrip sesi dan status proses. Yang **ditulis** hanya `~/.config/nakama`.
- Untuk menampilkan sisa kuota, nakama membaca token OAuth Claude Code dan memakainya **hanya** untuk satu permintaan ke `https://api.anthropic.com/api/oauth/usage` (paling sering sekali semenit per akun). Token tidak disimpan, ditampilkan, atau dicatat. Endpoint ini tidak didokumentasikan; `--offline` mematikannya.
- nakama menjalankan claude dengan `CLAUDE_CODE_FORCE_SYNC_OUTPUT=1` agar chat tidak berkedip.
- Format transkrip, berkas status, dan variabel di atas tidak resmi; versi Claude Code baru bisa mengubahnya.
- Judul sesi muncul di notifikasi, jadi hati-hati saat berbagi layar.

## Tema kantor (buat sendiri)

Repo ini **tidak menyertakan gambar apa pun**. Buat tema di `~/.config/nakama/themes/default/`
(atau nama lain, dipakai lewat `--theme`):

```
default/
├── theme.json        # salin dari examples/theme.json lalu sesuaikan
├── background.png    # denah dilihat dari atas, PNG
└── sprites/
    ├── captain/      # dihasilkan oleh import-sheet
    └── cook/
```

### 1. Lembar sprite

Satu PNG per karakter, latar **magenta polos `#FF00FF`**, tanpa teks atau garis, **6 baris** yang
terpisah jelas (minimal 12 piksel kosong antarbaris, 20 piksel antarframe):

| Baris | Isi | Frame |
|---|---|---|
| 1 | jalan ke bawah | 4 |
| 2 | jalan ke kiri (dibuat ulang sebagai cermin baris 3) | 4 |
| 3 | jalan ke kanan | 4 |
| 4 | jalan ke atas | 4 |
| 5 | tidur | 2 |
| 6 | bekerja | 4 |

```sh
nakama theme import-sheet --sheet captain.png --theme ~/.config/nakama/themes/default --name captain
```

### 2. `theme.json`

Koordinat `x` dan `y` adalah pecahan 0..1 dari gambar latar; titiknya adalah posisi **kaki** karakter.

| Kunci | Arti |
|---|---|
| `background.file`, `background.crop` | gambar latar; `crop` opsional `[x, y, lebar, tinggi]` |
| `charHeight` | tinggi karakter relatif terhadap tinggi latar |
| `characters` | nama → folder sprite |
| `agentTypes` | jenis agen → karakter; `default` wajib |
| `sessionCharacters` | opsional: satu karakter per sesi; yang pertama selalu dipakai lebih dulu |
| `heights` | opsional: tinggi per karakter (1 = biasa) |
| `nodes`, `edges`, `entrance` | jalur yang dilewati karakter; semua node harus tersambung |
| `stations` | tempat `work` atau `rest`; `for` berisi nama karakter atau jenis agen yang diutamakan |

Butuh minimal satu stasiun `work` dan satu `rest`.

### 3. Periksa

```sh
nakama theme preview --theme ~/.config/nakama/themes/default --out preview.png
```

Pastikan kamu berhak memakai gambar yang kamu pasang.

## Pengembangan

```sh
go vet ./... && go test ./... -race
```

## Lisensi

[MIT](LICENSE)
