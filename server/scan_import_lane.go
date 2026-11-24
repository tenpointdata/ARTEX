package server

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/scanimport"
)

// Code-security lane (E10): helpers that put a scanner import into the task's
// exploration graph. See db/code_findings.go for the graph writes.

// maxAnchorAssets caps asset_ids on one import.
const maxAnchorAssets = 20

// errAnchorsNeedTask is a 400: anchors join the exploration graph, which is
// per task.
var errAnchorsNeedTask = errors.New("asset_ids needs task_id: anchors join the task's exploration graph")

// parseAnchorAssetIDs reads "1,2,3" into distinct positive ids.
func parseAnchorAssetIDs(v string) ([]int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	if len(parts) > maxAnchorAssets {
		return nil, fmt.Errorf("asset_ids takes at most %d ids", maxAnchorAssets)
	}
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("bad asset_ids: want comma-separated asset ids")
		}
		if !containsID(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// laneLabel is the reader-facing name of a lane.
func laneLabel(l scanimport.Lane) string {
	switch l {
	case scanimport.LaneSAST:
		return "code scan (SAST)"
	case scanimport.LaneSCA:
		return "dependency scan (SCA)"
	}
	return "runtime scan"
}

// factNode is the import's exploration fact id, 0 when it has none.
func factNode(im *db.ScanImport) int64 {
	if im.FactNodeID == nil {
		return 0
	}
	return *im.FactNodeID
}

// importFactPayload is the exploration fact for one scan run. It holds typed
// values only (format, tool and version tokens, counts, file hash): the
// planner reads fact summaries, and nothing here comes from scanner free text.
func importFactPayload(im *db.ScanImport, res *scanimport.Result) map[string]any {
	lanes := map[string]int{}
	for _, f := range res.Findings {
		lanes[string(f.Lane)]++
	}
	var parts []string
	for _, l := range scanimport.Lanes() {
		if n := lanes[string(l)]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, laneLabel(l)))
		}
	}
	counts := "no findings"
	if len(parts) > 0 {
		counts = strings.Join(parts, ", ")
	}
	tool := im.ToolName
	if im.ToolVersion != "" {
		tool += " " + im.ToolVersion
	}
	return map[string]any{
		"summary": fmt.Sprintf("Scanner import #%d: %s output (%s), %d findings reported (%s), %d assets; imported results are unverified claims",
			im.ID, im.Format, tool, len(res.Findings), counts, len(res.Assets)),
		"source": db.ImportSourceOrigin, "import_id": im.ID, "format": im.Format,
		"tool": im.ToolName, "tool_version": im.ToolVersion, "file_sha256": im.FileSHA256,
		"findings": len(res.Findings), "assets": len(res.Assets), "lanes": lanes,
		"truncated": res.Truncated,
	}
}
