package main

// wire_upload.go — thin wrappers over internal/upload.

import (
	"io"

	"tetora/internal/upload"
)

// UploadedFile is an alias for upload.File for backward compatibility.
type UploadedFile = upload.File

func initUploadDir(baseDir string) string { return upload.InitDir(baseDir) }

func saveUpload(uploadDir, originalName string, reader io.Reader, size int64, source string) (*UploadedFile, error) {
	return upload.Save(uploadDir, originalName, reader, size, source)
}

func sanitizeFilename(name string) string  { return upload.SanitizeFilename(name) }
func detectMimeType(name string) string    { return upload.DetectMimeType(name) }

func buildFilePromptPrefix(files []*UploadedFile) string {
	return upload.BuildPromptPrefix(files)
}

func cleanupUploads(uploadDir string, days int) { upload.Cleanup(uploadDir, days) }

// coalesce returns the first non-empty string from the arguments.
// (Used by telegram.go and other root-level files.)
func coalesce(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
