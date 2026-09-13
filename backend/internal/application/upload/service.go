package applicationupload

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"
)

// Result 是一次上传的对外结果。
type Result struct {
	URL      string
	Kind     string
	Filename string
	Size     int64
}

// Observer 观测上传与媒体处理耗时，可选注入；未注入时不采集。
type Observer interface {
	ObserveUpload(kind string, duration time.Duration, err error)
	ObserveVideoProcessing(step string, duration time.Duration, err error)
}

// Service 编排「校验 -> 落盘 -> 探测 -> 转码」全流程。
type Service struct {
	storage  Storage
	media    MediaProcessor
	observer Observer
}

type Option func(*Service)

// WithObserver 注入耗时观测端口。
func WithObserver(observer Observer) Option {
	return func(s *Service) {
		s.observer = observer
	}
}

func New(storage Storage, media MediaProcessor, options ...Option) *Service {
	service := &Service{storage: storage, media: media}
	for _, option := range options {
		option(service)
	}
	return service
}

// Save 校验并保存上传文件；视频会额外做元数据校验与 faststart 处理，任一步失败都会回滚已落盘文件。
func (s *Service) Save(ctx context.Context, kind string, file *multipart.FileHeader) (result *Result, err error) {
	start := time.Now()
	defer func() {
		if s.observer != nil {
			s.observer.ObserveUpload(kind, time.Since(start), err)
		}
	}()

	validation, err := ValidateFile(file, kind)
	if err != nil {
		return nil, err
	}

	filename := s.storage.NewFilename(validation.Ext)
	saved, err := s.storage.Save(ctx, kind, filename, file)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageFailed, err)
	}

	if kind == "video" {
		if err := s.processVideo(ctx, saved.Path, validation.Ext); err != nil {
			_ = s.storage.Remove(ctx, saved)
			return nil, err
		}
	}

	return &Result{
		URL:      saved.URL,
		Kind:     kind,
		Filename: filename,
		Size:     saved.Size,
	}, nil
}

func (s *Service) processVideo(ctx context.Context, path string, ext string) error {
	probeStart := time.Now()
	metadata, err := s.media.Probe(ctx, path)
	if err != nil {
		s.observeProcessing("probe", probeStart, err)
		return err
	}
	err = ValidateMetadata(metadata)
	s.observeProcessing("probe", probeStart, err)
	if err != nil {
		return err
	}
	if !ShouldFaststart(ext) {
		return nil
	}

	faststartStart := time.Now()
	err = s.media.Faststart(ctx, path)
	s.observeProcessing("faststart", faststartStart, err)
	return err
}

func (s *Service) observeProcessing(step string, start time.Time, err error) {
	if s.observer == nil {
		return
	}
	s.observer.ObserveVideoProcessing(step, time.Since(start), err)
}
