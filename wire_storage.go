package main

// wire_storage.go wires the storage internal package to the root package.

import (
	"tetora/internal/storage"
)

// --- Service type aliases ---

type FileManagerService = storage.Service
type ManagedFile = storage.ManagedFile

// --- Constructors ---

func newFileManagerService(cfg *Config) *FileManagerService {
	dir := cfg.FileManager.storageDirOrDefault(cfg.baseDir)
	return storage.New(cfg.HistoryDB, dir, cfg.FileManager.maxSizeOrDefault(), makeLifeDB(), newUUID)
}

func initFileManagerDB(dbPath string) error {
	return storage.InitDB(dbPath)
}

// --- Forwarding helpers ---

func contentHash(data []byte) string {
	return storage.ContentHash(data)
}

func mimeFromExt(filename string) string {
	return storage.MimeFromExt(filename)
}
