package main

import "tetora/internal/backup"

func shouldInclude(relPath string) bool { return backup.ShouldInclude(relPath) }
func createBackup(baseDir, outputPath string) error { return backup.Create(baseDir, outputPath) }
func restoreBackup(backupPath, targetDir string) error { return backup.Restore(backupPath, targetDir) }
func listBackupContents(backupPath string) ([]string, error) { return backup.ListContents(backupPath) }
