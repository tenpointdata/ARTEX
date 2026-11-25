package server

import (
	"errors"
	"net/http"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/traffic"
)

// trafficIDsRequest is the body of the by-id traffic actions: the exchange ids
// of a row selection.
type trafficIDsRequest struct {
	IDs []string `json:"ids"`
}

// decodeTrafficIDs reads and normalizes a trafficIDsRequest, writing the 400 /
// 413 itself and returning ok=false when the request cannot proceed.
func decodeTrafficIDs(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var req trafficIDsRequest
	if err := decode(w, r, &req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return nil, false
		}
		writeErr(w, http.StatusBadRequest, "invalid body")
		return nil, false
	}
	ids := traffic.NormalizeIDs(req.IDs)
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "missing ids")
		return nil, false
	}
	if len(ids) > traffic.MaxBulkIDs {
		writeErr(w, http.StatusBadRequest, traffic.ErrTooManyIDs.Error())
		return nil, false
	}
	return ids, true
}

// bulkDeleteTraffic deletes exactly the exchanges named in {"ids": [...]} — the
// list's row selection — rather than every exchange of their hosts. Unknown ids
// are ignored; the response reports how many rows were deleted. Audited.
func (s *Server) bulkDeleteTraffic(w http.ResponseWriter, r *http.Request) {
	tr := s.m.Traffic()
	if tr == nil {
		writeErr(w, http.StatusNotFound, "traffic disabled")
		return
	}
	ids, ok := decodeTrafficIDs(w, r)
	if !ok {
		return
	}
	n, err := tr.DeleteIDs(ids)
	// Audited even on a partial failure: rows may already be gone.
	s.appendAuditBestEffort(actorName(r), db.AuditTrafficBulkDelete, "traffic", map[string]any{
		"requested": len(ids), "deleted": n, "ids": ids, "error": errString(err),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

// bulkExportTrafficHAR exports exactly the exchanges named in {"ids": [...]}
// as a HAR 1.2 download, with the same caps and X-ARTEX-HAR-* headers as the
// filter export. Raw exchanges carry credentials, so the export is audited.
func (s *Server) bulkExportTrafficHAR(w http.ResponseWriter, r *http.Request) {
	tr := s.m.Traffic()
	if tr == nil {
		writeErr(w, http.StatusServiceUnavailable, "traffic store is not available")
		return
	}
	ids, ok := decodeTrafficIDs(w, r)
	if !ok {
		return
	}
	doc, meta, err := tr.ExportHAR(traffic.PageQuery{IDs: ids, RespMin: -1, RespMax: -1},
		traffic.HARExportOptions{CreatorVersion: BuildVersion})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.appendAuditBestEffort(actorName(r), db.AuditTrafficBulkExport, "traffic", map[string]any{
		"requested": len(ids), "exported": meta.Exported, "matched": meta.TotalMatched,
		"truncated": meta.Truncated, "ids": ids,
	})
	writeHARDownload(w, doc, meta)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
