package backend

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupShouldSkipTransientPath(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"b3cb26ca-1.snapshot-restore-backup-abc/Default/Preferences", true},
		{"b3cb26ca-1.snapshot-restore-backup-abc", true},
		{"b3cb26ca-1.snapshot-restore-staging-abc/Default", true},
		{"b3cb26ca-1.profile-package-backup-abc/x", true},
		{".imports/batch-1/payload.zip", true},
		{".imports", true},
		{"b3cb26ca-1/Default/Preferences", false},
		{"b3cb26ca-1/Default/Extensions/abc/1.0_0/manifest.json", false},
		{"snapshots/b3cb26ca-1/snap.zip", false},
		{"extension-backups/b3cb26ca-1/abc/20260914.zip", false},
		{"imports/batch-1/payload.zip", false},
		{"", false},
		{"./", false},
	}

	for _, tc := range cases {
		if got := backupShouldSkipTransientPath(tc.rel); got != tc.want {
			t.Fatalf("backupShouldSkipTransientPath(%q) = %v, 期望 %v", tc.rel, got, tc.want)
		}
	}
}

// 导出备份时必须跳过恢复/导入产生的临时目录，否则备份体积会成倍增长。
func TestBackupZipAddDirSkipsTransientDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "user-data")
	writeTestFile(t, filepath.Join(root, "profile-1", "Default", "Preferences"), "keep")
	writeTestFile(t, filepath.Join(root, "profile-1.snapshot-restore-backup-abc", "Default", "Preferences"), "junk")
	writeTestFile(t, filepath.Join(root, "profile-2.snapshot-restore-staging-abc", "Default", "Preferences"), "junk")
	writeTestFile(t, filepath.Join(root, "profile-3.profile-package-backup-abc", "Default", "Preferences"), "junk")
	writeTestFile(t, filepath.Join(root, ".imports", "batch-1", "payload.zip"), "junk")

	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("创建压缩包失败: %v", err)
	}
	writer := zip.NewWriter(file)

	stats, err := backupZipAddDir(writer, root, "payload/browser/user-data/", zipPath, zipPath+".meta.json", backupShouldSkipTransientPath)
	if err != nil {
		t.Fatalf("写入目录失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭压缩包写入器失败: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("关闭压缩包失败: %v", err)
	}

	if stats.fileCount != 1 {
		t.Fatalf("期望只写入 1 个文件，实际 %d", stats.fileCount)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("打开压缩包失败: %v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, f := range reader.File {
		names = append(names, f.Name)
	}

	expected := "payload/browser/user-data/profile-1/Default/Preferences"
	if !containsZipEntry(names, expected) {
		t.Fatalf("正常数据未被写入，期望包含 %q，实际 %v", expected, names)
	}
	for _, name := range names {
		if backupShouldSkipTransientPath(strings.TrimPrefix(name, "payload/browser/user-data/")) {
			t.Fatalf("临时目录被写进了备份: %s", name)
		}
	}
}

func containsZipEntry(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
