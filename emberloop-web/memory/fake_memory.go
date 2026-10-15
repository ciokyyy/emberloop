package memory

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func (f *Fake) insertMemory(m MemoryItem) (int64, error) {
	if m.Status == "" {
		m.Status = StatusActive
	}
	if m.Veracity == "" {
		m.Veracity = VeracityInferred
	}
	if m.MentionCount == 0 {
		m.MentionCount = 1
	}
	if m.Importance == 0 {
		m.Importance = 0.7
	}
	if err := validMemory(m); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextMem++
	m.ID = f.nextMem
	if m.CreatedAt.IsZero() {
		m.CreatedAt = f.now()
	}
	if m.LastSeenAt.IsZero() {
		m.LastSeenAt = f.now()
	}
	f.mems = append(f.mems, m)
	return m.ID, nil
}
func (f *Fake) AddMemory(_ context.Context, m MemoryItem) (int64, error)    { return f.insertMemory(m) }
func (f *Fake) InsertMemory(_ context.Context, m MemoryItem) (int64, error) { return f.insertMemory(m) }
func (f *Fake) GetMemory(_ context.Context, id int64) (MemoryItem, error)   { return f.getMemory(id) }
func (f *Fake) MemoryByID(_ context.Context, id int64) (MemoryItem, error)  { return f.getMemory(id) }
func (f *Fake) getMemory(id int64) (MemoryItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.mems {
		if m.ID == id {
			return m, nil
		}
	}
	return MemoryItem{}, fmt.Errorf("%w: memory %d", ErrNotFound, id)
}
func (f *Fake) ListMemories(_ context.Context, kind, status string, limit int) ([]MemoryItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := clampList(limit)
	var out []MemoryItem
	for _, m := range f.mems {
		if kind != "" && m.Kind != kind {
			continue
		}
		if status != "" && m.Status != status {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}
func (f *Fake) SetMemoryStatus(_ context.Context, id int64, status string) error {
	if !validMemoryStatus(status) {
		return fmt.Errorf("%w: status %q", ErrInvalid, status)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.mems {
		if f.mems[i].ID == id {
			f.mems[i].Status = status
			return nil
		}
	}
	return fmt.Errorf("%w: memory %d", ErrNotFound, id)
}
func (f *Fake) PinMemory(_ context.Context, id int64, pinned int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.mems {
		if f.mems[i].ID == id {
			f.mems[i].Pinned = pinned
			return nil
		}
	}
	return fmt.Errorf("%w: memory %d", ErrNotFound, id)
}
func (f *Fake) SetMemoryPinned(_ context.Context, id int64, pinned bool) error {
	v := 0
	if pinned {
		v = 1
	}
	return f.PinMemory(nil, id, v)
}
func (f *Fake) BumpMemory(_ context.Context, id int64) (MemoryItem, error) { return f.bumpMemory(id) }
func (f *Fake) BumpMemoryMention(_ context.Context, id int64) (MemoryItem, error) {
	return f.bumpMemory(id)
}
func (f *Fake) bumpMemory(id int64) (MemoryItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.mems {
		if f.mems[i].ID == id {
			f.mems[i].MentionCount++
			f.mems[i].LastSeenAt = f.now()
			return f.mems[i], nil
		}
	}
	return MemoryItem{}, fmt.Errorf("%w: memory %d", ErrNotFound, id)
}
func (f *Fake) SetMemoryEmbedding(_ context.Context, id int64, embedding []byte, model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.mems {
		if f.mems[i].ID == id {
			f.mems[i].Embedding = embedding
			f.mems[i].EmbeddingModel = model
			return nil
		}
	}
	return fmt.Errorf("%w: memory %d", ErrNotFound, id)
}

func (f *Fake) insertEpisode(e Episode) (int64, error) {
	if e.Importance == 0 {
		e.Importance = 0.65
	}
	if err := validEpisode(e); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextEp++
	e.ID = f.nextEp
	if e.CreatedAt.IsZero() {
		e.CreatedAt = f.now()
	}
	f.eps = append(f.eps, e)
	return e.ID, nil
}
func (f *Fake) AddEpisode(_ context.Context, e Episode) (int64, error)    { return f.insertEpisode(e) }
func (f *Fake) InsertEpisode(_ context.Context, e Episode) (int64, error) { return f.insertEpisode(e) }
func (f *Fake) GetEpisode(_ context.Context, id int64) (Episode, error)   { return f.getEpisode(id) }
func (f *Fake) EpisodeByID(_ context.Context, id int64) (Episode, error)  { return f.getEpisode(id) }
func (f *Fake) getEpisode(id int64) (Episode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.eps {
		if e.ID == id {
			return e, nil
		}
	}
	return Episode{}, fmt.Errorf("%w: episode %d", ErrNotFound, id)
}
func (f *Fake) ListEpisodes(_ context.Context, limit int) ([]Episode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := clampList(limit)
	out := append([]Episode(nil), f.eps...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}
func (f *Fake) SetEpisodeEmbedding(_ context.Context, id int64, embedding []byte, model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.eps {
		if f.eps[i].ID == id {
			f.eps[i].Embedding = embedding
			f.eps[i].EmbeddingModel = model
			return nil
		}
	}
	return fmt.Errorf("%w: episode %d", ErrNotFound, id)
}
func (f *Fake) SetEpisodeConsolidated(_ context.Context, id int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.eps {
		if f.eps[i].ID == id {
			f.eps[i].ConsolidatedAt = at
			return nil
		}
	}
	return fmt.Errorf("%w: episode %d", ErrNotFound, id)
}
