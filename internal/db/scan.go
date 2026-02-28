package db

type cellScanner interface {
	Scan(dest ...any) error
}

func scanCell(row cellScanner) (*Cell, error) {
	var cell Cell
	var evalRequestedInt int
	var evalRanInt int
	if err := row.Scan(
		&cell.ID,
		&cell.Sequence,
		&cell.ParentID,
		&cell.Timestamp,
		&cell.Message,
		&cell.Source,
		&cell.Agent,
		&cell.Tags,
		&cell.Branch,
		&cell.FilesAdded,
		&cell.FilesModified,
		&cell.FilesRemoved,
		&cell.LinesAdded,
		&cell.LinesRemoved,
		&cell.TotalLOC,
		&cell.LOCDelta,
		&cell.TotalFiles,
		&evalRequestedInt,
		&evalRanInt,
		&cell.TestsPassed,
		&cell.TestsFailed,
		&cell.LintErrors,
		&cell.TypeErrors,
		&cell.EvalSkipped,
		&cell.EvalError,
	); err != nil {
		return nil, err
	}
	cell.EvalRequested = evalRequestedInt == 1
	cell.EvalRan = evalRanInt == 1
	return &cell, nil
}
