package tui

import (
	"path/filepath"

	"ero/internal/core"
)

func (m Model) reviewContextForLoadedReview(mode core.DiffMode) core.ReviewContext {
	ctx := m.reviewContext
	ctx.Target = reviewTargetForRequest(m.request, mode, ctx.Repository)
	ctx.Diff = core.DiffMetadata{FilesChanged: len(m.files)}
	ctx.Files = reviewFileMetadataFromFiles(m.files, &ctx.Diff)
	return ctx
}

func reviewTargetForRequest(request core.ReviewRequest, mode core.DiffMode, repo core.RepositoryMetadata) core.ReviewTargetMetadata {
	target := core.ReviewTargetMetadata{Mode: mode, BaseRef: request.BaseRevision, HeadRef: request.HeadRevision}
	switch mode {
	case core.DiffModeBranch:
		target.BaseRef = request.BaseRevision
		if target.BaseRef == "" {
			target.BaseRef = repo.DefaultBranch
		}
		target.HeadRef = repo.CurrentBranch
		target.HeadSHA = repo.HeadSHA
	case core.DiffModeCommit:
		target.HeadRef = request.Revision
		if target.HeadRef == "" {
			target.HeadRef = "HEAD"
		}
		target.HeadSHA = repo.HeadSHA
	case core.DiffModeRange:
		target.BaseRef = request.BaseRevision
		target.HeadRef = request.HeadRevision
	case core.DiffModeUpstream:
		target.BaseRef = request.UpstreamRef
		if target.BaseRef == "" {
			target.BaseRef = "@{upstream}"
		}
		target.HeadRef = "HEAD"
		target.HeadSHA = repo.HeadSHA
	}
	return target
}

func reviewFileMetadataFromFiles(files []core.ReviewFile, diff *core.DiffMetadata) []core.ReviewFileMetadata {
	metadata := make([]core.ReviewFileMetadata, 0, len(files))
	for _, file := range files {
		status := file.Status
		if status == "" {
			status = core.ReviewFileStatusModified
		}
		meta := core.ReviewFileMetadata{Path: file.Path, OldPath: file.OldPath, Status: status, Language: languageFromReviewPath(file.Path)}
		for _, section := range file.Sections {
			if section.Kind == core.SectionKindChanged {
				anchor := core.ReviewHunkAnchor{SectionID: section.ID}
				for _, line := range section.VisibleLines() {
					if anchor.OldStartLine == 0 && line.OldLineNumber > 0 {
						anchor.OldStartLine = line.OldLineNumber
					}
					if anchor.NewStartLine == 0 && line.NewLineNumber > 0 {
						anchor.NewStartLine = line.NewLineNumber
					}
					if anchor.OldStartLine > 0 && anchor.NewStartLine > 0 {
						break
					}
				}
				meta.Hunks = append(meta.Hunks, anchor)
			}
			for _, line := range section.VisibleLines() {
				if line.Kind == core.LineKindAdded {
					diff.Additions++
				}
				if line.Kind == core.LineKindDeleted {
					diff.Deletions++
				}
				meta.LineAnchors = append(meta.LineAnchors, core.NewReviewLineAnchor(file.Path, line))
			}
		}
		metadata = append(metadata, meta)
	}
	return metadata
}

func languageFromReviewPath(path string) string {
	ext := filepath.Ext(path)
	if len(ext) > 1 {
		return ext[1:]
	}
	return ""
}
