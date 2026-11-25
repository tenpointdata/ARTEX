package server

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	pgdb "github.com/Autumn-27/artex/db"
	"github.com/klauspost/compress/zstd"
)

// Archives made before cost_cells existed (c-f4a69d) are reported by the cost
// view as archived_omitted outside All by task/profile. Their packages still
// hold the cold llm_usage rows, so on start the server reads each such
// package's manifest once and records the cells (c-888292). The pass is
// idempotent: an archive leaves the candidate list once it has cells, so later
// starts cost one query.

// backfillArchiveCostCells runs the pass and reports how many archives gained
// cells. A package that cannot be read is logged and left for the next start.
func (s *Server) backfillArchiveCostCells() (int, error) {
	candidates, err := s.m.pg.ArchivesMissingCostCells()
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	scratch := filepath.Join(taskArchiveRoot(s.m.dir), ".backfill")
	if err := os.RemoveAll(scratch); err != nil {
		return 0, err
	}
	defer os.RemoveAll(scratch)
	done := 0
	for _, c := range candidates {
		if s.ctx.Err() != nil {
			return done, s.ctx.Err()
		}
		rows, err := s.archivedLLMUsage(c, scratch)
		if err != nil {
			log.Printf("[task-archive] cost backfill: archive %d: %v", c.ArchiveID, err)
			continue
		}
		wrote, err := s.m.pg.BackfillArchiveCostCells(c.ArchiveID, rows)
		if err != nil {
			log.Printf("[task-archive] cost backfill: archive %d: %v", c.ArchiveID, err)
			continue
		}
		if wrote {
			done++
		} else {
			log.Printf("[task-archive] cost backfill: archive %d: no usage rows in the package; keeping its per-profile totals", c.ArchiveID)
		}
	}
	log.Printf("[task-archive] cost backfill: %d of %d archives made before cost_cells now carry them", done, len(candidates))
	return done, nil
}

// archivedLLMUsage returns the llm_usage rows of one stored package, fetching
// it into scratch first when it lives in object storage.
func (s *Server) archivedLLMUsage(c pgdb.CostCellsBackfill, scratch string) (json.RawMessage, error) {
	if err := s.validateStoredArchive(c.ArchivePath); err != nil {
		return nil, err
	}
	path := c.ArchivePath
	if strings.HasPrefix(path, "s3://") {
		if s.archiveStore == nil {
			return nil, errors.New("package is in object storage, which is not configured")
		}
		if err := os.MkdirAll(scratch, archiveDirMode); err != nil {
			return nil, err
		}
		path = filepath.Join(scratch, fmt.Sprintf("%d.tar.zst", c.ArchiveID))
		defer os.Remove(path)
		if err := s.archiveStore.Download(s.ctx, c.ArchivePath, path, c.SHA256, c.CompressedSize); err != nil {
			return nil, err
		}
	}
	return readTaskArchiveUsageRows(path, c.SHA256, c.TaskID)
}

// readTaskArchiveUsageRows streams a package to its manifest.json and returns
// tables.llm_usage without extracting anything to disk. Other tables are
// skipped by the decoder, not held in memory.
func readTaskArchiveUsageRows(path, expectedSHA string, taskID int64) (json.RawMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := verifyTaskArchiveChecksum(file, expectedSHA); err != nil {
		return nil, err
	}
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	tarReader := tar.NewReader(decoder)
	for entries := 0; entries < maxArchiveFiles; entries++ {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("package has no manifest.json")
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || filepath.Clean(filepath.FromSlash(header.Name)) != "manifest.json" {
			continue
		}
		var manifest struct {
			FormatVersion int   `json:"format_version"`
			TaskID        int64 `json:"task_id"`
			Tables        struct {
				LLMUsage json.RawMessage `json:"llm_usage"`
			} `json:"tables"`
		}
		if err := json.NewDecoder(io.LimitReader(tarReader, header.Size)).Decode(&manifest); err != nil {
			return nil, fmt.Errorf("read manifest: %w", err)
		}
		if manifest.TaskID != taskID || !pgdb.IsTaskArchiveFormatSupported(manifest.FormatVersion) {
			return nil, pgdb.ErrTaskArchiveFormatMismatch
		}
		return manifest.Tables.LLMUsage, nil
	}
	return nil, errors.New("task archive exceeds extraction safety limits")
}
