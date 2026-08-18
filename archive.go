package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Archive writes a gzipped tar of the mirror at dir to out, atomically: it
// writes to out+".tmp" first and renames over out only on success, so a
// crash mid-write never leaves a truncated archive where a good one was.
//
// Every entry is written under a top-level directory named after dir's own
// base name, so "tar xzf out" leaves a self-contained "<name>.git/" a
// caller can git clone directly rather than scattering the mirror's
// contents into whatever directory they happened to extract into.
//
// Entries are read through an os.Root rooted at dir, which is what stops a
// symlink inside the mirror from making the walk reference anything outside
// it. Every entry's ModTime, Uid, Gid, Uname and Gname are zeroed, and
// entries are written in sorted path order, so archiving an unchanged
// mirror twice produces a byte-identical file -- which is what makes
// rsync or deduplication of the backup tree cheap.
func Archive(dir, out string) error {
	tmp := out + ".tmp"
	if err := writeArchive(dir, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("archive %s: %w", dir, err)
	}
	if err := os.Rename(tmp, out); err != nil {
		return fmt.Errorf("archive %s: %w", dir, err)
	}
	return nil
}

// epoch is the fixed ModTime every archived entry carries, so two archives
// of an unchanged mirror are byte-identical regardless of when they were
// written.
var epoch = time.Unix(0, 0).UTC()

func writeArchive(dir, out string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	paths, err := sortedPaths(root)
	if err != nil {
		return err
	}

	f, err := os.Create(out) //nolint:gosec // out is this package's own destination path, built from opts.ArchiveDir and a repo's namespace, not attacker input
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	prefix := filepath.Base(dir)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     prefix + "/",
		Mode:     0o755,
		ModTime:  epoch,
	}); err != nil {
		return err
	}

	for _, p := range paths {
		if err := writeEntry(tw, root, prefix, p); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}

func sortedPaths(root *os.Root) ([]string, error) {
	var paths []string
	err := fs.WalkDir(root.FS(), ".", func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

func writeEntry(tw *tar.Writer, root *os.Root, prefix, p string) error {
	info, err := root.Lstat(p)
	if err != nil {
		return err
	}

	var link string
	if info.Mode()&fs.ModeSymlink != 0 {
		link, err = fs.ReadLink(root.FS(), p)
		if err != nil {
			return err
		}
	}

	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	header.Name = prefix + "/" + p
	header.ModTime = epoch
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}
	header.Uid, header.Gid = 0, 0
	header.Uname, header.Gname = "", ""

	if info.IsDir() {
		header.Name += "/"
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return nil
	}

	src, err := root.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	_, err = io.Copy(tw, src)
	return err
}
