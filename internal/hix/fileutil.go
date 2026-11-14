package hix

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic ghi data vào path theo kiểu atomic: tạo thư mục cha (0700) nếu chưa có,
// ghi file tạm cùng thư mục, chmod perm, sync rồi rename đè lên path.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}

	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
