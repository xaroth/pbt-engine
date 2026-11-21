package dataset

import (
	"fmt"
	"io/fs"
	"sync"
)

// Progress is how far Load is. Each file counts once when read and once when decoded.
type Progress struct {
	// Total is the size of the files Load reads, in bytes.
	Total int64
	// Read and Decoded are the sizes of the files read and of the files decoded so far.
	Read, Decoded int64
	// File is the file just read or decoded; empty in the first report.
	File string
}

// LoadError is the error of the file whose group Load could not load.
type LoadError struct {
	File string
	Err  error
}

func (e *LoadError) Error() string { return fmt.Sprintf("load %s: %v", e.File, e.Err) }

func (e *LoadError) Unwrap() error { return e.Err }

// fileRead is a file Load reads, done once read.
type fileRead struct {
	done chan struct{}
	b    []byte
	err  error
}

// loadTask loads the group of file, or builds an index when file is empty.
type loadTask struct {
	file string
	run  func() error
}

// Load loads every group the manifest lists, with the text and names of language only, and
// builds the skill and item support indexes, so that no getter reads a file afterward. It reads
// every file at once and decodes each as soon as it and the groups it is checked against are
// in. It does not build Derived values.
//
// progress, when not nil, is called once before anything is read and after each file is read
// and after it is decoded, one call at a time, ending with Decoded equal to Total when Load
// succeeds. It is called from other goroutines than Load's.
//
// Load fails with a *LoadError for the first file in manifest order whose group failed to load.
// Since a Store keeps a group's first result, a Store that failed to load fails the same way
// again; open a new one to retry.
func (s *Store) Load(language string, progress func(Progress)) error {
	tasks := s.loadTasks(language)
	reads := map[string]*fileRead{}
	var p Progress
	for _, t := range tasks {
		if t.file != "" {
			reads[t.file] = &fileRead{done: make(chan struct{})}
			p.Total += int64(s.files[t.file].Size)
		}
	}
	var mu sync.Mutex
	report := func(file string, read, decoded int) {
		if progress == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		p.File = file
		p.Read += int64(read)
		p.Decoded += int64(decoded)
		progress(p)
	}
	report("", 0, 0)
	s.reads.Store(&reads)
	defer s.reads.Store(nil)

	var wg sync.WaitGroup
	for name, r := range reads {
		wg.Go(func() {
			r.b, r.err = fs.ReadFile(s.fsys, name)
			close(r.done)
			report(name, s.files[name].Size, 0)
		})
	}
	errs := make([]error, len(tasks))
	for i, t := range tasks {
		wg.Go(func() {
			errs[i] = t.run()
			if t.file != "" {
				report(t.file, 0, s.files[t.file].Size)
			}
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return &LoadError{File: tasks[i].file, Err: err}
		}
	}
	return nil
}

// loadTasks returns what Load runs: a task per file the manifest lists, in its order, skipping
// the texts and names of other languages than language, then the item supports index.
func (s *Store) loadTasks(language string) []loadTask {
	var tasks []loadTask
	single := s.singleLoads()
	for _, f := range s.manifest.Files {
		run, ok := single[f.Name]
		if !ok {
			run = s.loadFile(f.Name, language)
		}
		if run != nil {
			tasks = append(tasks, loadTask{f.Name, run})
		}
	}
	_, hasMods := s.files[ModsFile]
	if _, hasSkills := s.files[SkillsFile]; hasMods && hasSkills {
		tasks = append(tasks, loadTask{"", func() error { _, err := s.ItemGrants(); return err }})
	}
	return tasks
}

// singleLoads returns what loads each group of one file per dataset, by file.
func (s *Store) singleLoads() map[string]func() error {
	return map[string]func() error{
		StatsFile:      func() error { _, err := s.statsGroup(); return err },
		CharactersFile: func() error { _, err := s.Characters(); return err },
		ConstantsFile:  func() error { _, err := s.Constants(); return err },
		QuestsFile:     func() error { _, err := s.Quests(); return err },
		ItemsFile:      func() error { _, err := s.Items(); return err },
		ModsFile:       func() error { _, err := s.Mods(); return err },
		SkillsFile:     func() error { _, err := s.SkillIndex(); return err },
		BuffsFile:      func() error { _, err := s.Buffs(); return err },
		MonstersFile:   func() error { _, err := s.Monsters(); return err },
	}
}

// loadFile returns what loads the group of file name, of a group with one file per variant or
// language, or nil for a text or names file of another language than language.
func (s *Store) loadFile(name, language string) func() error {
	switch {
	case isGroup(name, treePrefix):
		v := groupTag(name, treePrefix)
		return func() error {
			if _, err := s.Tree(v); err != nil {
				return err
			}
			_, err := s.Classes(v)
			return err
		}
	case isGroup(name, layoutPrefix):
		return func() error { _, err := s.Layout(groupTag(name, layoutPrefix)); return err }
	case name == TextFile(language):
		return func() error { _, err := s.Text(language); return err }
	case name == NamesFile(language):
		return func() error { _, err := s.Names(language); return err }
	}
	return nil
}
