package catalog

import (
	"context"
	"sort"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

func (s *MemoryStore) UpsertTranscript(_ context.Context, transcript Transcript, segments []TranscriptSegment) (Transcript, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[transcript.AssetID]; !ok {
		return Transcript{}, ErrNotFound
	}
	now := time.Now().UTC()
	if transcript.ID == "" {
		transcript.ID = id.NewUUID()
	}
	if transcript.CreatedAt.IsZero() {
		transcript.CreatedAt = now
	}
	if transcript.Metadata == nil {
		transcript.Metadata = map[string]any{}
	}
	for i := range segments {
		if segments[i].ID == "" {
			segments[i].ID = id.NewUUID()
		}
		segments[i].TranscriptID = transcript.ID
		segments[i].AssetID = transcript.AssetID
		if segments[i].Metadata == nil {
			segments[i].Metadata = map[string]any{}
		}
	}
	transcript.Segments = append([]TranscriptSegment(nil), segments...)
	current := s.transcripts[transcript.AssetID]
	replaced := false
	for i := range current {
		if current[i].ID == transcript.ID {
			current[i] = transcript
			replaced = true
			break
		}
	}
	if !replaced {
		current = append(current, transcript)
	}
	s.transcripts[transcript.AssetID] = current
	return transcript, nil
}

func (s *MemoryStore) ListTranscripts(_ context.Context, assetID string, limit int) ([]Transcript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]Transcript(nil), s.transcripts[assetID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) ListAllTranscripts(_ context.Context, limit, offset int) ([]Transcript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Transcript{}
	for _, items := range s.transcripts {
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if offset < 0 {
		offset = 0
	}
	if offset > len(out) {
		return []Transcript{}, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) DeleteTranscript(_ context.Context, transcriptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for assetID, items := range s.transcripts {
		next := items[:0]
		deleted := false
		for _, item := range items {
			if item.ID == transcriptID {
				deleted = true
				continue
			}
			next = append(next, item)
		}
		if deleted {
			s.transcripts[assetID] = append([]Transcript(nil), next...)
			return nil
		}
	}
	return ErrNotFound
}

func (s *MemoryStore) UpsertAudioFeatures(_ context.Context, features AudioFeatures) (AudioFeatures, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[features.AssetID]; !ok {
		return AudioFeatures{}, ErrNotFound
	}
	if features.CreatedAt.IsZero() {
		features.CreatedAt = time.Now().UTC()
	}
	if features.Metadata == nil {
		features.Metadata = map[string]any{}
	}
	s.audioFeatures[features.AssetID] = features
	return features, nil
}

func (s *MemoryStore) GetAudioFeatures(_ context.Context, assetID string) (AudioFeatures, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	features, ok := s.audioFeatures[assetID]
	if !ok {
		return AudioFeatures{}, ErrNotFound
	}
	return features, nil
}

func (s *MemoryStore) UpsertVideoFrameCaption(_ context.Context, caption VideoFrameCaption) (VideoFrameCaption, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[caption.AssetID]; !ok {
		return VideoFrameCaption{}, ErrNotFound
	}
	if caption.ID == "" {
		caption.ID = id.NewUUID()
	}
	if caption.CreatedAt.IsZero() {
		caption.CreatedAt = time.Now().UTC()
	}
	if caption.Metadata == nil {
		caption.Metadata = map[string]any{}
	}
	current := s.frameCaptions[caption.AssetID]
	replaced := false
	for i := range current {
		if current[i].ID == caption.ID {
			current[i] = caption
			replaced = true
			break
		}
	}
	if !replaced {
		current = append(current, caption)
	}
	s.frameCaptions[caption.AssetID] = current
	return caption, nil
}

func (s *MemoryStore) ListVideoFrameCaptions(_ context.Context, assetID string, limit int) ([]VideoFrameCaption, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]VideoFrameCaption(nil), s.frameCaptions[assetID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].TimestampMS < out[j].TimestampMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) UpsertDocumentText(_ context.Context, doc DocumentText) (DocumentText, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[doc.AssetID]; !ok {
		return DocumentText{}, ErrNotFound
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.Metadata == nil {
		doc.Metadata = map[string]any{}
	}
	s.documentText[doc.AssetID] = doc
	return doc, nil
}

func (s *MemoryStore) GetDocumentText(_ context.Context, assetID string) (DocumentText, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.documentText[assetID]
	if !ok {
		return DocumentText{}, ErrNotFound
	}
	return doc, nil
}
