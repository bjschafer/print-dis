package web

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bjschafer/print-dis/internal/core"
	"github.com/bjschafer/print-dis/internal/files"
	"github.com/bjschafer/print-dis/internal/store"
)

// MeasureOldFiles measures the stored models that have no size, those
// uploaded before uploads were measured, and then sizes each affected request
// that has no size yet from all its measured models, as the upload would have.
// The size change is recorded as made by the uploader of the request's first
// newly measured file. A model that cannot be measured is logged and left
// alone, so running it again retries it. It reports how many files were
// measured and how many requests were sized.
func MeasureOldFiles(ctx context.Context, st *store.Store, blobs files.Blobs, log *slog.Logger) (measured, sized int, err error) {
	fs, err := st.ListUnmeasuredFiles(ctx)
	if err != nil {
		return 0, 0, err
	}
	var reqs []int64
	uploader := map[int64]int64{}
	for _, f := range fs {
		d := measureFile(ctx, blobs, log, f.ObjectKey, f.Filename, f.SizeBytes)
		if d == nil {
			continue
		}
		if err := st.SetFileDims(ctx, f.ID, *d); err != nil {
			return measured, sized, err
		}
		measured++
		log.Info("measured model", "file", f.Filename, "request", f.RequestID, "size", dimsText(*d))
		if _, ok := uploader[f.RequestID]; !ok {
			uploader[f.RequestID] = f.UploadedByID
			reqs = append(reqs, f.RequestID)
		}
	}
	for _, id := range reqs {
		all, err := st.ListFiles(ctx, id)
		if err != nil {
			return measured, sized, err
		}
		var ds []core.Dims
		var names []string
		for _, f := range all {
			if f.Dims != nil {
				ds = append(ds, *f.Dims)
				names = append(names, f.Filename)
			}
		}
		d := core.Fits(ds...)
		if d == nil { // the files were removed meanwhile
			continue
		}
		summary := fmt.Sprintf("set size %s from %s", dimsText(*d), strings.Join(names, ", "))
		set, err := st.SetRequestSize(ctx, id, *d, true, uploader[id], summary)
		if err != nil {
			return measured, sized, err
		}
		if set {
			sized++
			log.Info("sized request", "request", id, "size", dimsText(*d))
		}
	}
	return measured, sized, nil
}
