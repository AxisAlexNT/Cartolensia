package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
)

type componentDefinition struct {
	Key           string
	Name          string
	Category      string
	LicenseName   string
	ProvenanceURL string
	SourceURL     string
	Expected      []string
	Kind          string
	Metadata      map[string]any
}

var defaultComponents = []componentDefinition{
	{Key: "ffmpeg", Name: "FFmpeg", Category: "tool", LicenseName: "GPL/LGPL depending on build flags", ProvenanceURL: "https://ffmpeg.org", Expected: []string{"ffmpeg", "bin/ffmpeg"}, Kind: "executable", Metadata: map[string]any{"redistribution_note": "Check configure flags; --enable-nonfree must not be redistributed."}},
	{Key: "ffprobe", Name: "FFprobe", Category: "tool", LicenseName: "GPL/LGPL depending on build flags", ProvenanceURL: "https://ffmpeg.org", Expected: []string{"ffprobe", "bin/ffprobe"}, Kind: "executable"},
	{Key: "tesseract", Name: "Tesseract OCR", Category: "ocr", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/tesseract-ocr/tesseract", Expected: []string{"tesseract", "bin/tesseract"}, Kind: "executable"},
	{Key: "tessdata-eng", Name: "Tesseract English language data", Category: "ocr", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/tesseract-ocr/tessdata", Expected: []string{"eng.traineddata", "tessdata/eng.traineddata"}, Kind: "tessdata", Metadata: map[string]any{"language": "eng"}},
	{Key: "tessdata-rus", Name: "Tesseract Russian language data", Category: "ocr", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/tesseract-ocr/tessdata", Expected: []string{"rus.traineddata", "tessdata/rus.traineddata"}, Kind: "tessdata", Metadata: map[string]any{"language": "rus"}},
	{Key: "tessdata-hye", Name: "Tesseract Armenian language data", Category: "ocr", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/tesseract-ocr/tessdata", Expected: []string{"hye.traineddata", "tessdata/hye.traineddata"}, Kind: "tessdata", Metadata: map[string]any{"language": "hye"}},
	{Key: "tessdata-chi-sim", Name: "Tesseract Chinese Simplified language data", Category: "ocr", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/tesseract-ocr/tessdata", Expected: []string{"chi_sim.traineddata", "tessdata/chi_sim.traineddata"}, Kind: "tessdata", Metadata: map[string]any{"language": "chi_sim"}},
	{Key: "tessdata-chi-tra", Name: "Tesseract Chinese Traditional language data", Category: "ocr", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/tesseract-ocr/tessdata", Expected: []string{"chi_tra.traineddata", "tessdata/chi_tra.traineddata"}, Kind: "tessdata", Metadata: map[string]any{"language": "chi_tra"}},
	{Key: "vmaf", Name: "VMAF/libvmaf", Category: "metric", LicenseName: "BSD-2-Clause for libvmaf; ffmpeg build may be GPL", ProvenanceURL: "https://github.com/Netflix/vmaf", Kind: "ffmpeg_filter"},
	{Key: "python-ai-venv", Name: "Python AI virtual environment", Category: "python", LicenseName: "Python packages vary; inspect package metadata", Expected: []string{".cartolensia/ai-venv/bin/python", "bin/python"}, Kind: "python_venv"},
	{Key: "torch-cuda", Name: "PyTorch CUDA runtime", Category: "python", LicenseName: "BSD-style PyTorch plus NVIDIA wheel terms", ProvenanceURL: "https://pytorch.org", Kind: "python_import", Metadata: map[string]any{"import": "torch", "cuda": true}},
	{Key: "torchvision", Name: "Torchvision", Category: "python", LicenseName: "BSD-3-Clause", ProvenanceURL: "https://pytorch.org/vision", Kind: "python_import", Metadata: map[string]any{"import": "torchvision"}},
	{Key: "facenet-pytorch", Name: "facenet-pytorch", Category: "python", LicenseName: "MIT package; biometric model provenance requires review", ProvenanceURL: "https://pypi.org/project/facenet-pytorch/", Kind: "python_import", Metadata: map[string]any{"import": "facenet_pytorch"}},
	{Key: "asr-faster-whisper", Name: "faster-whisper ASR runtime", Category: "python", LicenseName: "MIT package; Whisper model weights need separate provenance review", ProvenanceURL: "https://github.com/SYSTRAN/faster-whisper", Kind: "python_import", Metadata: map[string]any{"import": "faster_whisper", "modality": "audio_asr"}},
	{Key: "asr-ctranslate2", Name: "CTranslate2 ASR backend", Category: "python", LicenseName: "MIT", ProvenanceURL: "https://github.com/OpenNMT/CTranslate2", Kind: "python_import", Metadata: map[string]any{"import": "ctranslate2", "modality": "audio_asr"}},
	{Key: "audio-librosa", Name: "librosa audio analysis", Category: "python", LicenseName: "ISC", ProvenanceURL: "https://librosa.org", Kind: "python_import", Metadata: map[string]any{"import": "librosa", "modality": "audio_features"}},
	{Key: "audio-soundfile", Name: "SoundFile audio IO", Category: "python", LicenseName: "BSD-3-Clause", ProvenanceURL: "https://python-soundfile.readthedocs.io", Kind: "python_import", Metadata: map[string]any{"import": "soundfile", "modality": "audio_features"}},
	{Key: "music-basic-pitch", Name: "Basic Pitch audio-to-MIDI", Category: "python", LicenseName: "Apache-2.0", ProvenanceURL: "https://github.com/spotify/basic-pitch", Kind: "python_import", Metadata: map[string]any{"import": "basic_pitch", "modality": "music_midi", "cache_only_outputs": true}},
	{Key: "music-demucs", Name: "Demucs music stem separation", Category: "python", LicenseName: "MIT package; model weights require provenance review", ProvenanceURL: "https://github.com/facebookresearch/demucs", Kind: "python_import", Metadata: map[string]any{"import": "demucs", "modality": "music_stems", "cache_only_outputs": true, "on_demand": true}},
	{Key: "music-mt3", Name: "MT3 multi-instrument music transcription", Category: "model", LicenseName: "Apache-2.0 code/model provenance review", ProvenanceURL: "https://github.com/magenta/mt3", Kind: "model_dir", Metadata: map[string]any{"modality": "music_midi", "optional": true, "status": "future_provider"}},
	{Key: "document-pymupdf", Name: "PyMuPDF document extraction", Category: "python", LicenseName: "AGPL-3.0-or-later; distribution needs review", ProvenanceURL: "https://pymupdf.readthedocs.io", Kind: "python_import", Metadata: map[string]any{"import": "fitz", "modality": "document_text"}},
	{Key: "efficientnet-b0", Name: "Torchvision EfficientNet-B0 weights", Category: "model", LicenseName: "Torchvision/ImageNet weights provenance requires review", ProvenanceURL: "https://docs.pytorch.org/vision/main/models/generated/torchvision.models.efficientnet_b0", Kind: "model_path", Expected: []string{".cartolensia/models/torch/hub/checkpoints/efficientnet_b0_rwightman-7f5810bc.pth", ".cartolensia/models/torch/hub/checkpoints/efficientnet_b0-355c32eb.pth"}},
	{Key: "mobilenetv3-large", Name: "Torchvision MobileNetV3 Large weights", Category: "model", LicenseName: "Torchvision/ImageNet weights provenance requires review", ProvenanceURL: "https://docs.pytorch.org/vision/stable/models/generated/torchvision.models.mobilenet_v3_large.html", Kind: "model_path", Expected: []string{".cartolensia/models/torch/hub/checkpoints/mobilenet_v3_large-8738ca79.pth"}},
	{Key: "opencv-yunet", Name: "OpenCV YuNet face detector", Category: "model", LicenseName: "Apache-2.0 OpenCV model provenance", ProvenanceURL: "https://github.com/opencv/opencv_zoo", Kind: "model_path", Expected: []string{".cartolensia/models/opencv/face_detection_yunet_2023mar.onnx", "opencv/face_detection_yunet_2023mar.onnx"}},
	{Key: "falconsai-nsfw", Name: "Falconsai NSFW image detection", Category: "model", LicenseName: "Apache-2.0 model card; labels are predictions", ProvenanceURL: "https://huggingface.co/Falconsai/nsfw_image_detection", Kind: "model_dir", Expected: []string{".cartolensia/models/huggingface/models--Falconsai--nsfw_image_detection"}},
	{Key: "openclip-vit-b32", Name: "OpenCLIP ViT-B/32 LAION", Category: "model", LicenseName: "MIT package/model card; LAION provenance risk accepted for local use", ProvenanceURL: "https://huggingface.co/laion/CLIP-ViT-B-32-laion2B-s34B-b79K", Kind: "model_dir", Expected: []string{".cartolensia/models/openclip", ".cartolensia/models/huggingface/models--laion--CLIP-ViT-B-32-laion2B-s34B-b79K"}},
	{Key: "blip-base", Name: "Salesforce BLIP base captioning", Category: "model", LicenseName: "BSD-3-Clause model card; generated captions are suggestions", ProvenanceURL: "https://huggingface.co/Salesforce/blip-image-captioning-base", Kind: "model_dir", Expected: []string{".cartolensia/models/huggingface/models--Salesforce--blip-image-captioning-base"}},
	{Key: "asr-model-small", Name: "faster-whisper small model", Category: "model", LicenseName: "MIT package; OpenAI Whisper model weights/provenance require release review", ProvenanceURL: "https://huggingface.co/Systran/faster-whisper-small", Kind: "model_dir", Expected: []string{".cartolensia/models/faster-whisper/models--Systran--faster-whisper-small", ".cartolensia/models/faster-whisper/small"}},
	{Key: "asr-model-medium", Name: "faster-whisper medium model", Category: "model", LicenseName: "MIT package; OpenAI Whisper model weights/provenance require release review", ProvenanceURL: "https://huggingface.co/Systran/faster-whisper-medium", Kind: "model_dir", Expected: []string{".cartolensia/models/faster-whisper/models--Systran--faster-whisper-medium", ".cartolensia/models/faster-whisper/medium"}},
}

func (s *Server) seedDefaultComponents() {
	ctx := context.Background()
	for _, def := range defaultComponents {
		if _, err := s.deps.Store.GetComponent(ctx, def.Key); err == nil {
			continue
		}
		component := catalog.Component{
			Key:           def.Key,
			Name:          def.Name,
			Category:      def.Category,
			Status:        "missing",
			SourceType:    "system_path",
			SourceURL:     def.SourceURL,
			LicenseName:   def.LicenseName,
			ProvenanceURL: def.ProvenanceURL,
			Metadata:      cloneMap(def.Metadata),
		}
		_, _ = s.deps.Store.UpsertComponent(ctx, component)
	}
}

func (s *Server) handleComponents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		query := catalog.ComponentQuery{
			Category: r.URL.Query().Get("category"),
			Status:   r.URL.Query().Get("status"),
			Q:        r.URL.Query().Get("q"),
			Limit:    intQuery(r.URL.Query(), "limit", 200, 1, 500),
			Offset:   intQuery(r.URL.Query(), "offset", 0, 0, 100000),
		}
		components, err := s.deps.Store.ListComponents(r.Context(), query)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"components": components, "root": componentRoot(), "total": len(components)})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleComponentsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	components, err := s.deps.Store.ListComponents(r.Context(), catalog.ComponentQuery{Limit: 500})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	counts := map[string]int{}
	for _, component := range components {
		counts[component.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts, "components": components, "root": componentRoot()})
}

func (s *Server) handleComponentByKey(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/components/"), "/")
	if path == "" {
		writeError(w, http.StatusNotFound, errors.New("component key is required"))
		return
	}
	parts := strings.Split(path, "/")
	key := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		component, err := s.deps.Store.GetComponent(r.Context(), key)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, component)
		return
	}
	action := parts[1]
	switch action {
	case "check":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleComponentCheck(w, r, key)
	case "download":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleComponentDownload(w, r, key)
	case "provide-path":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleComponentProvidePath(w, r, key)
	case "provide-archive":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleComponentProvideArchive(w, r, key)
	case "enable", "disable":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleComponentEnableDisable(w, r, key, action == "enable")
	case "events":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		events, err := s.deps.Store.ListComponentEvents(r.Context(), key, intQuery(r.URL.Query(), "limit", 100, 1, 500))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown component action %q", action))
	}
}

func (s *Server) handleComponentCheck(w http.ResponseWriter, r *http.Request, key string) {
	if !s.requireWrite(w, r, "components.write") {
		return
	}
	job := s.createComponentJob(r.Context(), key, "component_check")
	component, err := s.checkComponent(r.Context(), key)
	status := jobs.StatusSucceeded
	if err != nil {
		status = jobs.StatusFailed
	}
	job.Status = status
	now := time.Now().UTC()
	job.StartedAt = &now
	job.FinishedAt = &now
	total := int64(1)
	job.ProgressTotal = &total
	job.ProgressCurrent = 1
	payload := map[string]any{"component_key": key, "action": "check", "status": string(status)}
	if err != nil {
		job.Error = err.Error()
		payload["error"] = err.Error()
	}
	job.Payload = payload
	_ = s.deps.Store.UpdateJob(r.Context(), job)
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "component": component, "status": string(status), "error": errorString(err)})
}

func (s *Server) handleComponentDownload(w http.ResponseWriter, r *http.Request, key string) {
	if !s.requireWrite(w, r, "components.write") {
		return
	}
	job := s.createComponentJob(r.Context(), key, "component_download")
	component, err := s.deps.Store.GetComponent(r.Context(), key)
	if err == nil && strings.TrimSpace(component.SourceURL) != "" {
		err = fmt.Errorf("network download is intentionally not executed by this local handler yet; provide a reviewed archive/path or run an explicit future downloader for %s", key)
	} else if err == nil {
		err = fmt.Errorf("component %s has no reviewed source_url; provide archive/path or configure a reviewed source first", key)
	}
	if err != nil {
		component = s.updateComponentStatus(r.Context(), key, "failed", "", "", err.Error(), map[string]any{"download_job_id": job.ID})
	}
	now := time.Now().UTC()
	job.Status = jobs.StatusFailed
	job.StartedAt = &now
	job.FinishedAt = &now
	total := int64(1)
	job.ProgressTotal = &total
	job.ProgressCurrent = 1
	job.Error = err.Error()
	job.Payload = map[string]any{"component_key": key, "action": "download", "status": "failed", "error": err.Error()}
	_ = s.deps.Store.UpdateJob(r.Context(), job)
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "component": component, "status": "failed", "error": err.Error()})
}

func (s *Server) handleComponentProvidePath(w http.ResponseWriter, r *http.Request, key string) {
	if !s.requireWrite(w, r, "components.write") {
		return
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	component, err := s.provideComponentPath(r.Context(), key, payload.Path, "system_path")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, component)
}

func (s *Server) handleComponentProvideArchive(w http.ResponseWriter, r *http.Request, key string) {
	if !s.requireWrite(w, r, "components.write") {
		return
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job := s.createComponentJob(r.Context(), key, "component_provide_archive")
	component, err := s.importComponentArchive(r.Context(), key, payload.Path)
	now := time.Now().UTC()
	job.StartedAt = &now
	job.FinishedAt = &now
	total := int64(1)
	job.ProgressTotal = &total
	job.ProgressCurrent = 1
	jobPayload := map[string]any{"component_key": key, "action": "provide_archive"}
	if err != nil {
		job.Status = jobs.StatusFailed
		job.Error = err.Error()
		jobPayload["error"] = err.Error()
		job.Payload = jobPayload
		_ = s.deps.Store.UpdateJob(r.Context(), job)
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job.Status = jobs.StatusSucceeded
	job.Payload = jobPayload
	_ = s.deps.Store.UpdateJob(r.Context(), job)
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "component": component})
}

func (s *Server) handleComponentEnableDisable(w http.ResponseWriter, r *http.Request, key string, enable bool) {
	if !s.requireWrite(w, r, "components.write") {
		return
	}
	component, err := s.deps.Store.GetComponent(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if enable {
		if component.Status == "disabled" {
			component.Status = "missing"
		}
	} else {
		component.Status = "disabled"
	}
	component, err = s.deps.Store.UpsertComponent(r.Context(), component)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, _ = s.deps.Store.AddComponentEvent(r.Context(), catalog.ComponentEvent{ComponentKey: key, Level: "info", Message: map[bool]string{true: "component enabled", false: "component disabled"}[enable]})
	writeJSON(w, http.StatusOK, component)
}

func (s *Server) createComponentJob(ctx context.Context, key, kind string) jobs.Job {
	total := int64(1)
	job := jobs.Job{
		ID:            id.NewUUID(),
		Kind:          kind,
		Status:        jobs.StatusQueued,
		Payload:       map[string]any{"component_key": key},
		ProgressTotal: &total,
		MaxAttempts:   1,
		CreatedAt:     time.Now().UTC(),
	}
	created, err := s.deps.Store.EnqueueJob(ctx, job)
	if err == nil {
		return created
	}
	return job
}

func (s *Server) checkComponent(ctx context.Context, key string) (catalog.Component, error) {
	def, ok := componentDefinitionByKey(key)
	if !ok {
		return catalog.Component{}, catalog.ErrNotFound
	}
	component, err := s.deps.Store.GetComponent(ctx, key)
	if err != nil {
		component = catalog.Component{Key: def.Key, Name: def.Name, Category: def.Category, SourceType: "system_path", LicenseName: def.LicenseName, ProvenanceURL: def.ProvenanceURL, Metadata: cloneMap(def.Metadata)}
	}
	now := time.Now().UTC()
	component.LastCheckedAt = &now
	component.Error = ""
	component.Metadata = mergeMaps(cloneMap(def.Metadata), component.Metadata)
	switch def.Kind {
	case "executable":
		path, version, meta, err := checkExecutable(def)
		if err != nil {
			component.Status = "missing"
			component.Error = err.Error()
		} else {
			component.Status = "installed"
			component.SourceType = "system_path"
			component.ExecutablePath = path
			component.InstallPath = filepath.Dir(path)
			component.Version = version
			component.Metadata = mergeMaps(component.Metadata, meta)
			component.InstalledAt = &now
		}
	case "tessdata":
		lang := stringFromAny(def.Metadata["language"])
		ok, meta := tesseractLanguageAvailable(lang)
		component.Metadata = mergeMaps(component.Metadata, meta)
		if ok {
			component.Status = "installed"
			component.SourceType = "system_path"
			component.Version = "available"
			component.InstalledAt = &now
		} else {
			component.Status = "missing"
			component.Error = "Tesseract language data is not visible in tesseract --list-langs"
		}
	case "ffmpeg_filter":
		ok, meta := ffmpegFilterAvailable("libvmaf")
		component.Metadata = mergeMaps(component.Metadata, meta)
		if ok {
			component.Status = "installed"
			component.Version = "libvmaf filter available"
			component.SourceType = "system_path"
			component.InstalledAt = &now
		} else {
			component.Status = "missing"
			component.Error = "ffmpeg libvmaf filter is unavailable"
		}
	case "python_venv":
		path := filepath.Join(".cartolensia", "ai-venv", "bin", "python")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			component.Status = "installed"
			component.SourceType = "user_provided"
			component.ExecutablePath = path
			component.InstallPath = filepath.Dir(filepath.Dir(path))
			component.Version = runVersion(path, "--version")
			component.InstalledAt = &now
		} else {
			component.Status = "missing"
			component.Error = "repo-local AI virtualenv python is missing"
		}
	case "python_import":
		module := stringFromAny(def.Metadata["import"])
		ok, version, meta := pythonImportAvailable(module)
		component.Metadata = mergeMaps(component.Metadata, meta)
		if ok {
			component.Status = "installed"
			component.SourceType = "user_provided"
			component.Version = version
			component.InstalledAt = &now
		} else {
			component.Status = "missing"
			component.Error = "Python module is not importable from .cartolensia/ai-venv"
		}
	case "model_path", "model_dir":
		path, size, err := firstExistingExpected(def.Expected)
		if err != nil {
			component.Status = "missing"
			component.Error = err.Error()
		} else {
			component.Status = "installed"
			component.SourceType = "user_provided"
			component.InstallPath = path
			component.SizeBytes = size
			component.Version = "present"
			component.InstalledAt = &now
		}
	default:
		component.Status = "missing"
		component.Error = "component check type is not implemented"
	}
	component, err = s.deps.Store.UpsertComponent(ctx, component)
	if err != nil {
		return component, err
	}
	level := "info"
	msg := "component check completed"
	if component.Status == "missing" || component.Status == "failed" {
		level = "warn"
		msg = component.Error
	}
	_, _ = s.deps.Store.AddComponentEvent(ctx, catalog.ComponentEvent{ComponentKey: key, Level: level, Message: msg, Metadata: map[string]any{"status": component.Status}})
	return component, nil
}

func (s *Server) provideComponentPath(ctx context.Context, key, providedPath, sourceType string) (catalog.Component, error) {
	def, ok := componentDefinitionByKey(key)
	if !ok {
		return catalog.Component{}, catalog.ErrNotFound
	}
	cleanPath, err := safeOperatorPath(providedPath)
	if err != nil {
		return catalog.Component{}, err
	}
	if strings.HasPrefix(cleanPath, "/mnt/Models/rclone") {
		return catalog.Component{}, errors.New("component paths under /mnt/Models/rclone are forbidden")
	}
	if err := validateProvidedComponent(cleanPath, def); err != nil {
		component := s.updateComponentStatus(ctx, key, "failed", cleanPath, "", err.Error(), map[string]any{"source_type": sourceType})
		_, _ = s.deps.Store.AddComponentEvent(ctx, catalog.ComponentEvent{ComponentKey: key, Level: "error", Message: err.Error(), Metadata: map[string]any{"path": cleanPath}})
		return component, err
	}
	size := pathSize(cleanPath)
	checksum := ""
	if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
		checksum = sha256File(cleanPath)
	}
	now := time.Now().UTC()
	component, err := s.deps.Store.GetComponent(ctx, key)
	if err != nil {
		component = catalog.Component{Key: def.Key, Name: def.Name, Category: def.Category}
	}
	component.Status = "user_provided"
	component.SourceType = sourceType
	component.InstallPath = cleanPath
	component.ExecutablePath = executablePathFor(cleanPath, def)
	component.SizeBytes = size
	component.Checksum = checksum
	component.Error = ""
	component.InstalledAt = &now
	component.LastCheckedAt = &now
	component.LicenseName = nonEmpty(component.LicenseName, def.LicenseName)
	component.ProvenanceURL = nonEmpty(component.ProvenanceURL, def.ProvenanceURL)
	component.Metadata = mergeMaps(cloneMap(def.Metadata), component.Metadata)
	component, err = s.deps.Store.UpsertComponent(ctx, component)
	if err != nil {
		return catalog.Component{}, err
	}
	_, _ = s.deps.Store.AddComponentEvent(ctx, catalog.ComponentEvent{ComponentKey: key, Level: "info", Message: "user-provided component path accepted", Metadata: map[string]any{"path": cleanPath, "size_bytes": size}})
	return component, nil
}

func (s *Server) importComponentArchive(ctx context.Context, key, archivePath string) (catalog.Component, error) {
	def, ok := componentDefinitionByKey(key)
	if !ok {
		return catalog.Component{}, catalog.ErrNotFound
	}
	cleanArchive, err := safeOperatorPath(archivePath)
	if err != nil {
		return catalog.Component{}, err
	}
	if strings.HasPrefix(cleanArchive, "/mnt/Models/rclone") {
		return catalog.Component{}, errors.New("component archives under /mnt/Models/rclone are forbidden")
	}
	root := componentRoot()
	dest := filepath.Join(root, key)
	if !isPathInside(dest, root) {
		return catalog.Component{}, errors.New("component destination escaped component root")
	}
	if err := os.RemoveAll(dest); err != nil {
		return catalog.Component{}, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return catalog.Component{}, err
	}
	switch {
	case strings.HasSuffix(strings.ToLower(cleanArchive), ".zip"):
		err = extractZipSafe(cleanArchive, dest)
	case strings.HasSuffix(strings.ToLower(cleanArchive), ".tar.gz"), strings.HasSuffix(strings.ToLower(cleanArchive), ".tgz"):
		err = extractTarGzSafe(cleanArchive, dest)
	default:
		err = errors.New("unsupported archive type; supported: .zip, .tar.gz, .tgz")
	}
	if err != nil {
		_ = os.RemoveAll(dest)
		return catalog.Component{}, err
	}
	if err := validateProvidedComponent(dest, def); err != nil {
		_ = os.RemoveAll(dest)
		return catalog.Component{}, err
	}
	component, err := s.provideComponentPath(ctx, key, dest, "user_archive")
	if err == nil {
		component.Checksum = sha256File(cleanArchive)
		component.Metadata = mergeMaps(component.Metadata, map[string]any{"archive_path": cleanArchive, "archive_checksum": component.Checksum})
		component, err = s.deps.Store.UpsertComponent(ctx, component)
	}
	return component, err
}

func componentDefinitionByKey(key string) (componentDefinition, bool) {
	for _, def := range defaultComponents {
		if def.Key == key {
			return def, true
		}
	}
	return componentDefinition{}, false
}

func componentRoot() string {
	configured := strings.TrimSpace(os.Getenv("CARTOLENSIA_COMPONENT_DIR"))
	if configured == "" {
		configured = filepath.Join(".cartolensia", "components")
	}
	root, err := filepath.Abs(configured)
	if err != nil {
		return configured
	}
	return root
}

func safeOperatorPath(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", errors.New("path is required")
	}
	clean, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	if strings.Contains(filepath.Clean(clean), string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", errors.New("path traversal is rejected")
	}
	if _, err := os.Stat(clean); err != nil {
		return "", err
	}
	return clean, nil
}

func validateProvidedComponent(root string, def componentDefinition) error {
	if len(def.Expected) == 0 {
		if _, err := os.Stat(root); err != nil {
			return err
		}
		return nil
	}
	for _, expected := range def.Expected {
		candidate := expected
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, expected)
		}
		if info, err := os.Stat(candidate); err == nil {
			if def.Kind == "executable" && info.IsDir() {
				continue
			}
			return nil
		}
	}
	if found := findExpectedBasename(root, def.Expected); found != "" {
		return nil
	}
	return fmt.Errorf("component %s did not contain expected files: %s", def.Key, strings.Join(def.Expected, ", "))
}

func executablePathFor(root string, def componentDefinition) string {
	if def.Kind != "executable" {
		return ""
	}
	for _, expected := range def.Expected {
		candidate := expected
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, expected)
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return findExpectedBasename(root, def.Expected)
}

func findExpectedBasename(root string, expected []string) string {
	names := map[string]struct{}{}
	for _, value := range expected {
		names[filepath.Base(value)] = struct{}{}
	}
	found := ""
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" || entry.IsDir() {
			return nil
		}
		if _, ok := names[entry.Name()]; ok {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func extractZipSafe(archivePath, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, file := range zr.File {
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("archive symlink rejected: %s", file.Name)
		}
		target, err := safeArchiveTarget(dest, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		_ = out.Close()
		_ = in.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func extractTarGzSafe(archivePath, dest string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("archive link rejected: %s", header.Name)
		}
		target, err := safeArchiveTarget(dest, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			_ = out.Close()
			if copyErr != nil {
				return copyErr
			}
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
	}
}

func safeArchiveTarget(dest, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("archive absolute path rejected: %s", name)
	}
	cleanName := filepath.Clean(name)
	if cleanName == "." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) || cleanName == ".." {
		return "", fmt.Errorf("archive traversal rejected: %s", name)
	}
	target := filepath.Join(dest, cleanName)
	if !isPathInside(target, dest) {
		return "", fmt.Errorf("archive target escaped destination: %s", name)
	}
	return target, nil
}

func isPathInside(pathValue, root string) bool {
	pathAbs, err := filepath.Abs(pathValue)
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func checkExecutable(def componentDefinition) (string, string, map[string]any, error) {
	name := filepath.Base(def.Expected[0])
	path, err := exec.LookPath(name)
	if err != nil {
		return "", "", nil, err
	}
	version := runVersion(path, "-version")
	meta := map[string]any{}
	if name == "ffmpeg" {
		meta["configure"] = ffmpegConfigureLine(path)
		meta["nonfree"] = strings.Contains(stringFromAny(meta["configure"]), "--enable-nonfree")
		meta["gpl"] = strings.Contains(stringFromAny(meta["configure"]), "--enable-gpl")
	}
	return path, version, meta, nil
}

func runVersion(binary string, arg string) string {
	out, err := exec.Command(binary, arg).CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return line
}

func ffmpegConfigureLine(binary string) string {
	out, err := exec.Command(binary, "-hide_banner", "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "configuration:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "configuration:"))
		}
	}
	return ""
}

func tesseractLanguageAvailable(lang string) (bool, map[string]any) {
	out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
	meta := map[string]any{"required_language": lang}
	if err != nil {
		meta["error"] = err.Error()
		return false, meta
	}
	lines := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of available") {
			continue
		}
		lines = append(lines, line)
		if line == lang {
			meta["available_languages"] = lines
			return true, meta
		}
	}
	meta["available_languages"] = lines
	return false, meta
}

func ffmpegFilterAvailable(filter string) (bool, map[string]any) {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-filters").CombinedOutput()
	meta := map[string]any{"filter": filter}
	if err != nil {
		meta["error"] = err.Error()
		return false, meta
	}
	available := strings.Contains(string(out), filter)
	meta["available"] = available
	return available, meta
}

func pythonImportAvailable(module string) (bool, string, map[string]any) {
	python := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_PYTHON"))
	if python == "" {
		var err error
		python, err = exec.LookPath("python3")
		if err != nil {
			python = filepath.Join(".cartolensia", "ai-venv", "bin", "python")
		}
	}
	script := "import importlib; m=importlib.import_module('" + module + "'); print(getattr(m, '__version__', 'installed'))"
	cmd := exec.Command(python, "-c", script)
	cmd.Env = os.Environ()
	meta := map[string]any{"module": module, "python": python}
	out, err := cmd.CombinedOutput()
	if err != nil {
		meta["error"] = strings.TrimSpace(string(out))
		return false, "", meta
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		version = "installed"
	}
	return true, version, meta
}

func firstExistingExpected(paths []string) (string, int64, error) {
	for _, value := range paths {
		for _, candidate := range componentExpectedCandidates(value) {
			if info, err := os.Stat(candidate); err == nil {
				if info.IsDir() {
					return candidate, pathSize(candidate), nil
				}
				return candidate, info.Size(), nil
			}
		}
	}
	return "", 0, fmt.Errorf("none of the expected paths exists: %s", strings.Join(paths, ", "))
}

func componentExpectedCandidates(value string) []string {
	var out []string
	add := func(path string) {
		path = filepath.Clean(path)
		for _, existing := range out {
			if existing == path {
				return
			}
		}
		out = append(out, path)
	}
	if filepath.IsAbs(value) {
		add(value)
		return out
	}
	if strings.HasPrefix(value, ".cartolensia/models/") {
		relative := strings.TrimPrefix(value, ".cartolensia/models/")
		for _, root := range []string{os.Getenv("CARTOLENSIA_MODEL_DIR"), os.Getenv("CARTOLENSIA_AI_MODEL_DIR")} {
			if strings.TrimSpace(root) != "" {
				add(filepath.Join(root, relative))
			}
		}
	}
	add(value)
	return out
}

func pathSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		_ = path
		return nil
	})
	return total
}

func sha256File(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Server) updateComponentStatus(ctx context.Context, key, status, installPath, executablePath, errText string, metadata map[string]any) catalog.Component {
	def, _ := componentDefinitionByKey(key)
	component, err := s.deps.Store.GetComponent(ctx, key)
	if err != nil {
		component = catalog.Component{Key: key, Name: def.Name, Category: def.Category, SourceType: "system_path", LicenseName: def.LicenseName, ProvenanceURL: def.ProvenanceURL}
	}
	now := time.Now().UTC()
	component.Status = status
	component.InstallPath = installPath
	component.ExecutablePath = executablePath
	component.Error = errText
	component.LastCheckedAt = &now
	component.Metadata = mergeMaps(component.Metadata, metadata)
	saved, saveErr := s.deps.Store.UpsertComponent(ctx, component)
	if saveErr == nil {
		return saved
	}
	return component
}

func mergeMaps(base map[string]any, extra map[string]any) map[string]any {
	out := cloneMap(base)
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sortedComponentKeys() []string {
	keys := make([]string, 0, len(defaultComponents))
	for _, def := range defaultComponents {
		keys = append(keys, def.Key)
	}
	sort.Strings(keys)
	return keys
}
