package workspace

import (
	"fmt"
	"strings"

	"github.com/kalyskhairy/nakama/internal/input"

	"github.com/charmbracelet/x/ansi"
)

// HelpLines is the help screen: every key (from input.Bindings, so it never drifts), what the
// marks and the status bar mean, how the screen works, where things are stored, and tips.
func HelpLines(width int) []string {
	var raw []string
	add := func(s ...string) { raw = append(raw, s...) }
	add("nakama — bantuan   (esc, q atau F1 menutup, ↑/↓ menggulir)", "")
	last := ""
	for _, b := range input.Bindings {
		if b.Context != last {
			add("", b.Context)
			last = b.Context
		}
		add(fmt.Sprintf("  %-16s %s", b.Keys, b.Desc))
	}
	add("", "Arti tanda",
		"  ?  menunggu izin atau jawabanmu     ●  bekerja     ◐  aktif     ○  diam     ✓  selesai",
		"  Di kantor, karakter gelap kebiruan adalah subagen; yang berwarna penuh adalah sesi chat.",
		"  (terminal lain)  sesi sedang dipakai di terminal lain: hanya bisa dilihat, tidak di-resume",
		"  baru saja aktif  transkrip berubah beberapa detik lalu tanpa proses yang dikenal: Enter meminta konfirmasi",
		"",
		"Bar status",
		"  ! N menunggu  jumlah agen (termasuk subagen) yang menunggu kamu — paling penting, dipotong terakhir",
		"  5j / 7h       pemakaian batas rencana 5 jam dan 7 hari, dengan jam reset; per akun bila lebih dari satu",
		"  tok           token aktif hari ini: hanya sesi di panel Aktif atau yang dibuka nakama",
		"",
		"Cara kerja",
		"  Semua sesi Claude Code di laptop ini dibaca dari ~/.claude*/projects, dikelompokkan per folder.",
		"  Folder yang ditampilkan dipilih dengan f; folder baru tersembunyi sampai kamu memilihnya.",
		"  Sesi yang menunggu atau berjalan tetap muncul di panel Aktif walau foldernya disembunyikan.",
		"  Enter pada sesi lama menjalankan `claude --resume` di folder dan akun aslinya. Sesi yang dipakai",
		"  terminal lain tidak di-resume agar dua proses tidak menulis ke satu transkrip.",
		"  Akun: ~/.claude adalah personal, ~/.claude-work adalah work. Sesi baru memakai akun yang paling",
		"  sering dipakai di folder itu; Tab di pemilih mengganti.",
		"",
		"Berkas dan flag",
		"  ~/.config/nakama/folders.json  pilihan folder     ~/.config/nakama/themes/  tema kantor",
		"  nakama --offline  tanpa panggilan jaringan (batas rencana tidak tampil)",
		"  --prefix N  ganti tombol fokus (byte; bawaan 0 = Ctrl+Space)   --theme NAMA   --graphics auto|sixel|off",
		"  --font LxT  ukuran sel terminal untuk kantor   --office-max-scale 1   --context-window 200000",
		"",
		"Tips",
		"  Shift+drag menyalin teks saat mouse aktif; m mematikan mouse sama sekali.",
		"  z memperbesar chat ke layar penuh; Ctrl+Space atau z kembali.",
		"  Di dalam chat semua tombol (termasuk esc) milik Claude, kecuali Ctrl+Space dan F1.")
	var out []string
	for _, l := range raw {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(l, max(width, 10), " "), "\n")...)
	}
	return out
}

// ScrollHelp is the page of lines starting at offset, clamped so the last page stays full.
func ScrollHelp(lines []string, offset, h int) []string {
	offset = max(min(offset, len(lines)-h), 0)
	return lines[offset:min(offset+h, len(lines))]
}
