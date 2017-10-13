package goodroutine

import (
	"os"
	"sync"
	"time"
)

// FileChangeRoutine implements an interval routine that calls a function on file change.
// A file change is detected when the OS reported file ModTime or size has changed.
// Some important notes:
// - the error interval is only triggered by an error returned by the function, not by file stat error
// - the first run of Stats on file does not trigger the function (not considered a change)
// - file stat error on a file only triggers a change once
type FileChangeRoutine struct {
	OnFileChange func(file string, stat os.FileInfo, err error)
	innerF       func() error
	files        []string
	fileStats    []os.FileInfo
	once         *sync.Once

	IntervalRoutine
}

// NewFileChangeRoutine creates a new FileChangeRoutine, which takes care of running f().
// Parameters are equivalent to IntervalRoutine.
func NewFileChangeRoutine(f func() error, runInterval time.Duration, retryInterval time.Duration) *FileChangeRoutine {
	fcr := &FileChangeRoutine{
		IntervalRoutine: IntervalRoutine{
			runInterval:   runInterval,
			retryInterval: retryInterval,
			force:         make(chan bool, 1),
			done:          make(chan bool, 1),
		},
		innerF: f,
		once:   &sync.Once{},
	}
	fcr.IntervalRoutine.f = func() error {
		return fcr.update()
	}
	return fcr
}

// AddFiles adds files to watch for updates.
// Parameter is a list of file paths, empty path are ignored.
// This function must be called prior to calling Start()
func (fcr *FileChangeRoutine) AddFiles(files ...string) {
	for _, file := range files {
		if file == "" {
			// ignore empty files for convenience
			continue
		}
		fcr.files = append(fcr.files, file)
		fcr.fileStats = append(fcr.fileStats, nil)
	}
}

func (fcr *FileChangeRoutine) update() error {
	change := false
	for i, file := range fcr.files {
		stat, err := os.Stat(file)
		ostat := fcr.fileStats[i]
		if err != nil {
			// error on stat, file probably does not exist or bad perm
			if ostat == nil {
				// no previous stat, dont trigger forever
				continue
			}
		}
		if ostat == nil || stat == nil || !stat.ModTime().Equal(ostat.ModTime()) || stat.Size() != ostat.Size() {
			if fcr.OnFileChange != nil {
				fcr.OnFileChange(file, stat, err)
			}
			change = true
			fcr.fileStats[i] = stat
		}
	}
	fcr.once.Do(func() {
		// dont trigger change on 1st run, it's not a change
		change = false
	})

	if !change {
		// no error, no file change
		return nil
	}
	return fcr.innerF()
}
