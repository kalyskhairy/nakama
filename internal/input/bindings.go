package input

// Binding documents one key or mouse action. The help screen is built from this table, so it
// cannot drift from what the keys do.
type Binding struct{ Context, Keys, Desc string }

var Bindings = []Binding{
	{"Daftar", "Tab", "pindah antara panel Aktif dan Folder/Sesi"},
	{"Daftar", "↑ / ↓ / j / k", "pilih baris sebelumnya atau berikutnya"},
	{"Daftar", "Enter", "folder: buka atau tutup; sesi: buka chat (sesi lama di-resume)"},
	{"Daftar", "z", "perbesar chat ke layar penuh"},
	{"Daftar", "f", "pilih folder yang ditampilkan"},
	{"Daftar", "a", "akun di pohon: semua, personal, work (panel Aktif tetap semua)"},
	{"Daftar", "n", "sesi baru di folder pilihan"},
	{"Daftar", "d", "detail sesi: 50 kejadian terakhir"},
	{"Daftar", "/", "saring folder dan sesi dengan teks (Enter selesai, esc hapus)"},
	{"Daftar", "v", "ganti ke kantor"},
	{"Daftar", "x", "tutup sesi nakama yang sudah selesai"},
	{"Daftar", "m", "matikan atau nyalakan mouse (mati: drag biasa menyalin teks)"},
	{"Daftar", "? / h", "bantuan ini"},
	{"Daftar", "q", "keluar (tekan dua kali bila masih ada sesi berjalan)"},
	{"Chat", "Ctrl+Space", "pindah fokus dari chat ke daftar (satu tombol; semua tombol lain milik Claude)"},
	{"Chat", "F1", "bantuan, juga dari dalam chat"},
	{"Zoom", "Ctrl+Space", "kembali ke tiga panel (z di dalam chat dikirim ke Claude)"},
	{"Kantor", "Tab / ← → ↑ ↓", "pilih agen"},
	{"Kantor", "i", "buka atau tutup kartu agen"},
	{"Kantor", "Enter", "bicara dengan agen terpilih di panel chat (jendela kecil: chat penuh)"},
	{"Kantor", "Ctrl+Space", "dari panel chat kembali memilih agen"},
	{"Kantor", "z", "chat layar penuh"},
	{"Kantor", "v / esc", "kembali ke tiga panel"},
	{"Filter folder", "Space", "tampilkan atau sembunyikan folder"},
	{"Filter folder", "a", "tampilkan semua folder"},
	{"Filter folder", "Enter / esc", "simpan / batal"},
	{"Sesi baru", "ketik", "saring folder atau tulis path (diawali / atau ~)"},
	{"Sesi baru", "Tab", "ganti akun (personal, work, ...)"},
	{"Sesi baru", "Enter / esc", "mulai / batal"},
	{"Mouse", "klik", "pilih baris, atau beri fokus ke panel"},
	{"Mouse", "klik dua kali", "sama dengan Enter"},
	{"Mouse", "roda", "gulir panel di bawah kursor"},
	{"Mouse", "Shift+drag", "menyalin teks (saat mouse aktif)"},
}
